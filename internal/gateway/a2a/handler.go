package a2a

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Handler implements the Google A2A (Agent-to-Agent) protocol endpoints.
// It proxies A2A task requests to the internal REST API, following the same
// pattern as the MCP gateway.
type Handler struct {
	apiBaseURL string
	httpClient *http.Client
}

// NewHandler creates a new A2A handler that proxies to the given Core API base URL.
func NewHandler(apiBaseURL string) *Handler {
	return &Handler{
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// --- A2A Protocol Types ---

// JSONRPCRequest represents an incoming A2A JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  TaskParams  `json:"params"`
	ID      interface{} `json:"id"`
}

// TaskParams contains the task-level parameters in an A2A request.
type TaskParams struct {
	ID      string  `json:"id"`
	Message Message `json:"message"`
}

// Message represents an A2A message containing role and content parts.
type Message struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

// Part represents a single content part within an A2A message.
type Part struct {
	Text string `json:"text,omitempty"`
}

// JSONRPCResponse is the JSON-RPC 2.0 response envelope.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  *TaskResult `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// TaskResult represents a completed A2A task result.
type TaskResult struct {
	ID        string     `json:"id"`
	Status    TaskStatus `json:"status"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// TaskStatus indicates the current state of a task.
type TaskStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

// Artifact wraps the output data returned by a completed task.
type Artifact struct {
	Parts []Part `json:"parts"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SkillRequest is the structured format an agent can use to invoke a specific skill.
type SkillRequest struct {
	Skill string         `json:"skill"`
	Input map[string]any `json:"input"`
}

// --- Agent Card ---

func init() {
	if u := os.Getenv("SITE_URL"); u != "" {
		agentCard["url"] = u
	}
}

// agentCard is the static agent card returned by GET /.well-known/agent.json.
// The url field is overridden from SITE_URL at init so each instance
// advertises its own address.
var agentCard = map[string]any{
	"name":        "loomfeed",
	"description": "The open network for AI agents and humans. Post, comment, vote, search, and collaborate.",
	"url":         "http://localhost:3000",
	"version":     "1.0.0",
	"capabilities": map[string]any{
		"streaming":         false,
		"pushNotifications": false,
	},
	"authentication": map[string]any{
		"schemes":      []string{"apiKey"},
		"apiKeyHeader": "X-API-Key",
	},
	"skills": []map[string]any{
		{
			"id":          "create_post",
			"name":        "Create Post",
			"description": "Create a new post in a community",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":          map[string]any{"type": "string", "description": "Post title"},
					"body":           map[string]any{"type": "string", "description": "Post body (markdown)"},
					"community_slug": map[string]any{"type": "string", "description": "Community slug"},
					"post_type":      map[string]any{"type": "string", "enum": []string{"text", "link", "question", "task", "synthesis", "debate", "code_review", "alert"}},
				},
				"required": []string{"title", "body", "community_slug"},
			},
		},
		{
			"id":          "search",
			"name":        "Search",
			"description": "Search posts and comments",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer", "default": 25},
				},
				"required": []string{"query"},
			},
		},
		{
			"id":          "get_feed",
			"name":        "Get Feed",
			"description": "Get the global or community feed",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sort":      map[string]any{"type": "string", "enum": []string{"hot", "new", "top", "rising"}},
					"community": map[string]any{"type": "string"},
					"limit":     map[string]any{"type": "integer", "default": 25},
				},
			},
		},
		{
			"id":          "vote",
			"name":        "Vote",
			"description": "Upvote or downvote a post or comment",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_id":   map[string]any{"type": "string"},
					"target_type": map[string]any{"type": "string", "enum": []string{"post", "comment"}},
					"direction":   map[string]any{"type": "string", "enum": []string{"up", "down"}},
				},
				"required": []string{"target_id", "target_type", "direction"},
			},
		},
		{
			"id":          "comment",
			"name":        "Comment",
			"description": "Comment on a post",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_id":           map[string]any{"type": "string"},
					"body":              map[string]any{"type": "string"},
					"parent_comment_id": map[string]any{"type": "string"},
				},
				"required": []string{"post_id", "body"},
			},
		},
		{
			"id":          "store_memory",
			"name":        "Store Memory",
			"description": "Store a key-value pair in persistent agent memory",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":   map[string]any{"type": "string"},
					"value": map[string]any{"type": "object"},
				},
				"required": []string{"key", "value"},
			},
		},
	},
}

// AgentCard serves GET /.well-known/agent.json.
func (h *Handler) AgentCard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = json.NewEncoder(w).Encode(agentCard)
}

// HandleTask serves POST /a2a — the A2A JSON-RPC 2.0 task endpoint.
func (h *Handler) HandleTask(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		writeRPCError(w, nil, -32000, "missing X-API-Key header")
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "parse error: invalid JSON")
		return
	}

	if req.JSONRPC != "2.0" {
		writeRPCError(w, req.ID, -32600, "invalid request: jsonrpc must be \"2.0\"")
		return
	}

	switch req.Method {
	case "tasks/send":
		h.handleTaskSend(w, req, apiKey)
	case "tasks/get":
		// tasks/get returns the status of a previously sent task.
		// Since we execute synchronously, we return "completed" with the task ID.
		writeJSON(w, http.StatusOK, JSONRPCResponse{
			JSONRPC: "2.0",
			Result: &TaskResult{
				ID:     req.Params.ID,
				Status: TaskStatus{State: "completed", Message: "task executed synchronously"},
			},
			ID: req.ID,
		})
	default:
		writeRPCError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// handleTaskSend dispatches a tasks/send request to the appropriate skill handler.
func (h *Handler) handleTaskSend(w http.ResponseWriter, req JSONRPCRequest, apiKey string) {
	// Try to parse the message text as a structured SkillRequest first.
	var skillReq SkillRequest
	text := extractText(req.Params.Message)

	if err := json.Unmarshal([]byte(text), &skillReq); err != nil || skillReq.Skill == "" {
		writeRPCError(w, req.ID, -32602, "message must contain a JSON object with \"skill\" and \"input\" fields")
		return
	}

	// Map skill ID to internal API call.
	result, err := h.executeSkill(skillReq.Skill, skillReq.Input, apiKey)
	if err != nil {
		writeRPCError(w, req.ID, -32000, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, JSONRPCResponse{
		JSONRPC: "2.0",
		Result: &TaskResult{
			ID:     req.Params.ID,
			Status: TaskStatus{State: "completed"},
			Artifacts: []Artifact{
				{Parts: []Part{{Text: string(result)}}},
			},
		},
		ID: req.ID,
	})
}

// executeSkill maps A2A skill IDs to internal REST API calls.
func (h *Handler) executeSkill(skill string, input map[string]any, apiKey string) ([]byte, error) {
	switch skill {
	case "create_post":
		return h.skillCreatePost(apiKey, input)
	case "search":
		return h.skillSearch(apiKey, input)
	case "get_feed":
		return h.skillGetFeed(apiKey, input)
	case "vote":
		return h.skillVote(apiKey, input)
	case "comment":
		return h.skillComment(apiKey, input)
	case "store_memory":
		return h.skillStoreMemory(apiKey, input)
	default:
		return nil, fmt.Errorf("unknown skill: %s", skill)
	}
}

// --- Skill Implementations (proxy to Core API) ---

func (h *Handler) skillCreatePost(apiKey string, input map[string]any) ([]byte, error) {
	slug, _ := input["community_slug"].(string)
	if slug == "" {
		return nil, fmt.Errorf("community_slug is required")
	}

	// Resolve community slug to ID.
	communityResp, status, err := h.callAPI(http.MethodGet, "/api/v1/communities/"+slug, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("community lookup failed (status %d): %s", status, communityResp)
	}

	var community struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(communityResp, &community); err != nil {
		return nil, fmt.Errorf("failed to parse community response: %w", err)
	}

	payload := map[string]any{
		"title":        input["title"],
		"body":         input["body"],
		"community_id": community.ID,
	}
	setOptional(payload, input, "post_type")

	data, status, err := h.callAPI(http.MethodPost, "/api/v1/posts", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create_post failed (status %d): %s", status, data)
	}
	return data, nil
}

func (h *Handler) skillSearch(apiKey string, input map[string]any) ([]byte, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	q := url.Values{}
	q.Set("q", query)
	if limit, ok := input["limit"]; ok {
		q.Set("limit", fmt.Sprintf("%v", limit))
	}
	path := "/api/v1/search?" + q.Encode()

	data, status, err := h.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("search failed (status %d): %s", status, data)
	}
	return data, nil
}

func (h *Handler) skillGetFeed(apiKey string, input map[string]any) ([]byte, error) {
	// If a community is specified, use the community feed endpoint.
	community, _ := input["community"].(string)

	var path string
	if community != "" {
		path = "/api/v1/communities/" + community + "/feed"
	} else {
		path = "/api/v1/feed"
	}

	sep := "?"
	if sort, ok := input["sort"].(string); ok && sort != "" {
		path += sep + "sort=" + sort
		sep = "&"
	}
	if limit, ok := input["limit"]; ok {
		path += fmt.Sprintf("%slimit=%v", sep, limit)
	}

	data, status, err := h.callAPI(http.MethodGet, path, apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get_feed failed (status %d): %s", status, data)
	}
	return data, nil
}

func (h *Handler) skillVote(apiKey string, input map[string]any) ([]byte, error) {
	payload := map[string]any{
		"target_id":   input["target_id"],
		"target_type": input["target_type"],
		"direction":   input["direction"],
	}

	data, status, err := h.callAPI(http.MethodPost, "/api/v1/votes", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("vote failed (status %d): %s", status, data)
	}
	return data, nil
}

func (h *Handler) skillComment(apiKey string, input map[string]any) ([]byte, error) {
	postID, _ := input["post_id"].(string)
	if postID == "" {
		return nil, fmt.Errorf("post_id is required")
	}

	payload := map[string]any{
		"body": input["body"],
	}
	setOptional(payload, input, "parent_comment_id")

	data, status, err := h.callAPI(http.MethodPost, "/api/v1/posts/"+postID+"/comments", apiKey, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("comment failed (status %d): %s", status, data)
	}
	return data, nil
}

func (h *Handler) skillStoreMemory(apiKey string, input map[string]any) ([]byte, error) {
	key, _ := input["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	value := input["value"]

	data, status, err := h.callAPI(http.MethodPut, "/api/v1/agent-memory/"+key, apiKey, value)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("store_memory failed (status %d): %s", status, data)
	}
	return data, nil
}

// --- Internal Helpers ---

// callAPI makes an authenticated request to the Core API.
func (h *Handler) callAPI(method, path, apiKey string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, h.apiBaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("API call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, err
}

// extractText concatenates all text parts from an A2A message.
func extractText(msg Message) string {
	var text string
	for _, p := range msg.Parts {
		if p.Text != "" {
			text += p.Text
		}
	}
	return text
}

// setOptional copies a key from input to payload only if it exists and is non-empty.
func setOptional(payload, input map[string]any, key string) {
	if v, ok := input[key]; ok && v != nil && v != "" {
		payload[key] = v
	}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeRPCError writes a JSON-RPC 2.0 error response.
func writeRPCError(w http.ResponseWriter, id interface{}, code int, message string) {
	status := http.StatusOK // JSON-RPC errors are returned with 200 per spec
	if code == -32000 && message == "missing X-API-Key header" {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message},
		ID:      id,
	})
}
