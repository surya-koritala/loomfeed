package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// newMockCoreAPI creates a test HTTP server that stubs Core API responses.
func newMockCoreAPI(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// callHandleToolCall posts a tool-call request to s.HandleToolCall and returns
// the recorder so the caller can inspect status and body.
func callHandleToolCall(t *testing.T, s *Server, apiKey, tool string, input map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(ToolCallRequest{Tool: tool, Input: input})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	rec := httptest.NewRecorder()
	s.HandleToolCall(rec, req)
	return rec
}

// TestServer_HandleToolCall_GetFeed verifies that the get_feed tool calls
// GET /api/v1/feed on the Core API.
func TestServer_HandleToolCall_GetFeed(t *testing.T) {
	const wantPath = "/api/v1/feed"
	var gotPath string

	mock := newMockCoreAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"posts":[]}`))
	})
	defer mock.Close()

	s := NewServer(mock.URL)
	rec := callHandleToolCall(t, s, "test-api-key", "get_feed", map[string]any{
		"sort":  "new",
		"limit": "10",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if !strings.HasPrefix(gotPath, wantPath) {
		t.Errorf("Core API path = %q, want prefix %q", gotPath, wantPath)
	}

	var resp ToolCallResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error field: %s", resp.Error)
	}
	if resp.Result == nil {
		t.Error("expected non-nil result")
	}
}

// TestServer_HandleToolCall_UnknownTool verifies that an unknown tool name
// results in a 400 response with an error message.
func TestServer_HandleToolCall_UnknownTool(t *testing.T) {
	s := NewServer("http://localhost:8080") // won't be called

	rec := callHandleToolCall(t, s, "test-api-key", "nonexistent_tool", nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp ToolCallResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(resp.Error, "nonexistent_tool") {
		t.Errorf("error %q does not mention tool name", resp.Error)
	}
}

// TestServer_HandleToolCall_MissingAPIKey verifies that a request without an
// X-API-Key header receives a 401 Unauthorized response.
func TestServer_HandleToolCall_MissingAPIKey(t *testing.T) {
	s := NewServer("http://localhost:8080") // won't be called

	// Pass empty string so callHandleToolCall skips setting the header.
	rec := callHandleToolCall(t, s, "", "get_feed", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp ToolCallResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error field in response")
	}
}

func TestNormalizeSources(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"nil", nil, nil},
		{"empty string", "", nil},
		{"native string slice", []string{"https://a", "https://b"}, []string{"https://a", "https://b"}},
		{"native any slice", []any{"https://a", "https://b"}, []string{"https://a", "https://b"}},
		{"json-array string", `["https://a","https://b"]`, []string{"https://a", "https://b"}},
		{"comma-separated", "https://a, https://b", []string{"https://a", "https://b"}},
		{"newline-separated", "https://a\nhttps://b", []string{"https://a", "https://b"}},
		{"mixed delimiters with whitespace", " https://a , \n https://b \r\n", []string{"https://a", "https://b"}},
		{"json string with empty entries", `["https://a","","https://b"," "]`, []string{"https://a", "https://b"}},
		{"dedupe", []any{"https://a", "https://a", "https://b"}, []string{"https://a", "https://b"}},
		{"malformed json falls through to delimiter parse", `[https://a, https://b`, []string{"[https://a", "https://b"}},
		{"unrelated type", 42, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeSources(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

