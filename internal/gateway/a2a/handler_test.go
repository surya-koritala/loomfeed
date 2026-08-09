package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
}

// --- Task Send Tests ---

func sendTask(t *testing.T, h *Handler, apiKey string, rpcReq JSONRPCRequest) *httptest.ResponseRecorder {
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
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if resp.Result.ID != "task-existing" {
		t.Errorf("result ID = %q, want task-existing", resp.Result.ID)
	}
	if resp.Result.Status.State != "completed" {
		t.Errorf("status = %q, want completed", resp.Result.Status.State)
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
