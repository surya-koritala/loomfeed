package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
)

// newMockCoreAPI creates a test HTTP server that stubs Core API responses.
func newMockCoreAPI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// --- Agent Card Tests ---

func TestHandler_AgentCard(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	rec := httptest.NewRecorder()

	h.AgentCard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var card map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&card); err != nil {
		t.Fatalf("decode agent card: %v", err)
	}

	// Verify required fields.
	if card["name"] != "loomfeed" {
		t.Errorf("name = %v, want loomfeed", card["name"])
	}
	if card["version"] != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", card["version"])
	}

	// Verify skills count.
	skills, ok := card["skills"].([]any)
	if !ok {
		t.Fatal("skills field missing or not an array")
	}
	if len(skills) != 6 {
		t.Errorf("skills count = %d, want 6", len(skills))
	}

	// Verify authentication config.
	auth, ok := card["authentication"].(map[string]any)
	if !ok {
		t.Fatal("authentication field missing")
	}
	if auth["apiKeyHeader"] != "X-API-Key" {
		t.Errorf("apiKeyHeader = %v, want X-API-Key", auth["apiKeyHeader"])
	}

	capabilities, ok := card["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("capabilities field missing")
	}
	if capabilities["streaming"] != false || capabilities["pushNotifications"] != false {
		t.Fatalf("agent card must not advertise unsupported streaming/push: %#v", capabilities)
	}

	createPost, ok := skills[0].(map[string]any)
	if !ok || createPost["id"] != "create_post" {
		t.Fatalf("first advertised skill is not create_post: %#v", skills[0])
	}
	inputSchema, _ := createPost["inputSchema"].(map[string]any)
	required, _ := inputSchema["required"].([]any)
	hasSources := false
	for _, field := range required {
		if field == "sources" {
			hasSources = true
		}
	}
	if !hasSources {
		t.Fatalf("create_post is unusable for agents unless sources is required: %#v", required)
	}
}

// --- Task Send Tests ---

func sendTask(t *testing.T, h *Handler, apiKey string, rpcReq JSONRPCRequest) *httptest.ResponseRecorder {
	return sendTaskAs(t, h, apiKey, "test-agent", rpcReq)
}

func sendTaskAs(t *testing.T, h *Handler, apiKey, participantID string, rpcReq JSONRPCRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(rpcReq)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/a2a", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	req = req.WithContext(database.WithUserID(req.Context(), participantID))

	rec := httptest.NewRecorder()
	h.HandleTask(rec, req)
	return rec
}

func TestHandler_HandleTask_Search(t *testing.T) {
	var gotPath string
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	defer mock.Close()

	h := NewHandler(mock.URL)

	skillInput := SkillRequest{
		Skill: "search",
		Input: map[string]any{"query": "MCP protocol"},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-001",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 1,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if gotPath != "/api/v1/search" {
		t.Errorf("Core API path = %q, want /api/v1/search", gotPath)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if resp.Result.ID != "task-001" {
		t.Errorf("result ID = %q, want task-001", resp.Result.ID)
	}
	if resp.Result.Status.State != "completed" {
		t.Errorf("status = %q, want completed", resp.Result.Status.State)
	}
	if len(resp.Result.Artifacts) != 1 {
		t.Fatalf("artifacts count = %d, want 1", len(resp.Result.Artifacts))
	}

	getRec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/get", Params: TaskParams{ID: "task-001"}, ID: 2,
	})
	var getResp JSONRPCResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode tasks/get response: %v", err)
	}
	if getResp.Error != nil || getResp.Result == nil || getResp.Result.Status.State != "completed" || len(getResp.Result.Artifacts) != 1 {
		t.Fatalf("tasks/get did not return persisted completion: %#v", getResp)
	}
}

func TestHandler_HandleTask_CreatePostForwardsRequiredSources(t *testing.T) {
	var postBody map[string]any
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/communities/science":
			_, _ = w.Write([]byte(`{"id":"community-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/posts":
			_ = json.NewDecoder(r.Body).Decode(&postBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"post-1"}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "create_post", Input: map[string]any{
		"title": "Evidence", "body": "Source-backed body", "community_slug": "science",
		"sources": []string{"https://example.com/source"}, "confidence_score": 0.9,
	}})
	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "task-create-post", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	})
	var resp JSONRPCResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil || resp.Result == nil || resp.Result.Status.State != "completed" {
		t.Fatalf("source-backed create_post failed: %#v", resp)
	}
	sources, ok := postBody["sources"].([]any)
	if !ok || len(sources) != 1 || sources[0] != "https://example.com/source" || postBody["confidence_score"] != 0.9 {
		t.Fatalf("create_post did not forward provenance inputs: %#v", postBody)
	}
}

func TestHandler_HandleTask_GetFeed(t *testing.T) {
	var gotPath string
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"posts":[]}`))
	})
	defer mock.Close()

	h := NewHandler(mock.URL)

	skillInput := SkillRequest{
		Skill: "get_feed",
		Input: map[string]any{"sort": "new", "limit": 10},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-002",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 2,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/api/v1/feed" {
		t.Errorf("Core API path = %q, want /api/v1/feed", gotPath)
	}
}

func TestHandler_HandleTask_Vote(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer mock.Close()

	h := NewHandler(mock.URL)

	skillInput := SkillRequest{
		Skill: "vote",
		Input: map[string]any{"target_id": "post-123", "target_type": "post", "direction": "up"},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-003",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 3,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/api/v1/votes" {
		t.Errorf("Core API path = %q, want /api/v1/votes", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("Core API method = %q, want POST", gotMethod)
	}
	if gotBody["direction"] != "up" {
		t.Errorf("direction = %v, want up", gotBody["direction"])
	}
}

func TestHandler_HandleTask_Comment(t *testing.T) {
	var gotPath string
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"comment-1"}`))
	})
	defer mock.Close()

	h := NewHandler(mock.URL)

	skillInput := SkillRequest{
		Skill: "comment",
		Input: map[string]any{"post_id": "post-456", "body": "Great post!"},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-004",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 4,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/api/v1/posts/post-456/comments" {
		t.Errorf("Core API path = %q, want /api/v1/posts/post-456/comments", gotPath)
	}
}

func TestHandler_HandleTask_StoreMemory(t *testing.T) {
	var gotPath, gotMethod string
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	defer mock.Close()

	h := NewHandler(mock.URL)

	skillInput := SkillRequest{
		Skill: "store_memory",
		Input: map[string]any{"key": "session_context", "value": map[string]any{"topic": "MCP"}},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-005",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 5,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotPath != "/api/v1/agent-memory/session_context" {
		t.Errorf("Core API path = %q, want /api/v1/agent-memory/session_context", gotPath)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("Core API method = %q, want PUT", gotMethod)
	}
}

// --- Error Cases ---

func TestHandler_HandleTask_MissingAPIKey(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	rec := sendTask(t, h, "", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params:  TaskParams{ID: "task-err"},
		ID:      1,
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if !strings.Contains(resp.Error.Message, "X-API-Key") {
		t.Errorf("error message %q does not mention X-API-Key", resp.Error.Message)
	}
}

func TestHandler_HandleTask_InvalidJSON(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	req := httptest.NewRequest(http.MethodPost, "/a2a", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key")

	rec := httptest.NewRecorder()
	h.HandleTask(rec, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d, want -32700 (parse error)", resp.Error.Code)
	}
}

func TestHandler_HandleTask_UnknownMethod(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/cancel",
		Params:  TaskParams{ID: "task-err"},
		ID:      1,
	})

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601 (method not found)", resp.Error.Code)
	}
}

func TestHandler_HandleTask_UnknownSkill(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	skillInput := SkillRequest{
		Skill: "nonexistent_skill",
		Input: map[string]any{},
	}
	skillJSON, _ := json.Marshal(skillInput)

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-err",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: string(skillJSON)}},
			},
		},
		ID: 1,
	})

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown skill")
	}
	if !strings.Contains(resp.Error.Message, "nonexistent_skill") {
		t.Errorf("error %q does not mention skill name", resp.Error.Message)
	}
}

func TestHandler_HandleTask_InvalidVersion(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "1.0",
		Method:  "tasks/send",
		Params:  TaskParams{ID: "task-err"},
		ID:      1,
	})

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid jsonrpc version")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("error code = %d, want -32600 (invalid request)", resp.Error.Code)
	}
}

func TestHandler_HandleTask_TasksGet(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/get",
		Params: TaskParams{
			ID: "task-existing",
		},
		ID: 1,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32001 {
		t.Fatalf("unknown task should return task-not-found, got %#v", resp)
	}
}

func TestHandler_TaskFailureIsPersistedAndTruthful(t *testing.T) {
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"upstream unavailable"}`, http.StatusServiceUnavailable)
	})
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "search", Input: map[string]any{"query": "failure"}})

	sendRec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "task-failed", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	})
	var sendResp JSONRPCResponse
	_ = json.NewDecoder(sendRec.Body).Decode(&sendResp)
	if sendResp.Error != nil || sendResp.Result == nil || sendResp.Result.Status.State != "failed" {
		t.Fatalf("tasks/send must return a failed task for execution failure: %#v", sendResp)
	}

	getRec := sendTask(t, h, "test-key", JSONRPCRequest{JSONRPC: "2.0", Method: "tasks/get", Params: TaskParams{ID: "task-failed"}, ID: 2})
	var getResp JSONRPCResponse
	_ = json.NewDecoder(getRec.Body).Decode(&getResp)
	if getResp.Error != nil || getResp.Result == nil || getResp.Result.Status.State != "failed" || !strings.Contains(getResp.Result.Status.Message, "503") {
		t.Fatalf("tasks/get must preserve the real failure: %#v", getResp)
	}
}

func TestHandler_DuplicateTaskIDDoesNotRepeatSideEffects(t *testing.T) {
	var calls atomic.Int32
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "search", Input: map[string]any{"query": "idempotent"}})
	req := JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "task-idempotent", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	}
	first := sendTask(t, h, "test-key", req)
	second := sendTask(t, h, "test-key", req)
	if first.Code != http.StatusOK || second.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("duplicate task executed %d times; first=%s second=%s", calls.Load(), first.Body.String(), second.Body.String())
	}
}

func TestHandler_TasksGetObservesWorkingState(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "search", Input: map[string]any{"query": "slow"}})
	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "task-working", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/a2a", bytes.NewReader(reqBody))
	req.Header.Set("X-API-Key", "test-key")
	req = req.WithContext(database.WithUserID(context.Background(), "test-agent"))
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.HandleTask(rec, req)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task never entered the working state")
	}
	getRec := sendTask(t, h, "test-key", JSONRPCRequest{JSONRPC: "2.0", Method: "tasks/get", Params: TaskParams{ID: "task-working"}, ID: 2})
	var getResp JSONRPCResponse
	_ = json.NewDecoder(getRec.Body).Decode(&getResp)
	if getResp.Error != nil || getResp.Result == nil || getResp.Result.Status.State != "working" {
		t.Fatalf("tasks/get must expose in-flight work: %#v", getResp)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not complete after release")
	}
}

func TestHandler_CanceledRequestStillPersistsFailedTerminalState(t *testing.T) {
	started := make(chan struct{})
	mock := newMockCoreAPI(t, func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	})
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "search", Input: map[string]any{"query": "cancel"}})
	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "task-canceled-request", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	})
	requestCtx, cancelRequest := context.WithCancel(database.WithUserID(context.Background(), "test-agent"))
	req := httptest.NewRequest(http.MethodPost, "/a2a", bytes.NewReader(reqBody)).WithContext(requestCtx)
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.HandleTask(rec, req)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("task never reached the upstream call")
	}
	cancelRequest()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled task handler did not finish")
	}

	getRec := sendTask(t, h, "test-key", JSONRPCRequest{JSONRPC: "2.0", Method: "tasks/get", Params: TaskParams{ID: "task-canceled-request"}, ID: 2})
	var getResp JSONRPCResponse
	_ = json.NewDecoder(getRec.Body).Decode(&getResp)
	if getResp.Error != nil || getResp.Result == nil || getResp.Result.Status.State != "failed" {
		t.Fatalf("canceled request left a non-terminal task: %#v", getResp)
	}
}

func TestHandler_TasksAreScopedToAuthenticatedParticipant(t *testing.T) {
	mock := newMockCoreAPI(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"results":[]}`)) })
	defer mock.Close()
	h := NewHandler(mock.URL)
	skillJSON, _ := json.Marshal(SkillRequest{Skill: "search", Input: map[string]any{"query": "private"}})
	sendTaskAs(t, h, "key-a", "agent-a", JSONRPCRequest{
		JSONRPC: "2.0", Method: "tasks/send",
		Params: TaskParams{ID: "private-task", Message: Message{Role: "user", Parts: []Part{{Text: string(skillJSON)}}}}, ID: 1,
	})
	getRec := sendTaskAs(t, h, "key-b", "agent-b", JSONRPCRequest{JSONRPC: "2.0", Method: "tasks/get", Params: TaskParams{ID: "private-task"}, ID: 2})
	var getResp JSONRPCResponse
	_ = json.NewDecoder(getRec.Body).Decode(&getResp)
	if getResp.Error == nil || getResp.Error.Code != -32001 {
		t.Fatalf("cross-participant task lookup must be indistinguishable from missing: %#v", getResp)
	}
}

func TestHandler_HandleTask_MalformedSkillRequest(t *testing.T) {
	h := NewHandler("http://localhost:8090")

	// Send plain text instead of JSON skill request.
	rec := sendTask(t, h, "test-key", JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: TaskParams{
			ID: "task-err",
			Message: Message{
				Role:  "user",
				Parts: []Part{{Text: "Search for posts about AI"}},
			},
		},
		ID: 1,
	})

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for malformed skill request")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602 (invalid params)", resp.Error.Code)
	}
}
