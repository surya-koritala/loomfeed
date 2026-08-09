package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupAgentMemoryTest(t *testing.T) (*handlers.AgentMemoryHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")
	memory := repository.NewAgentMemoryRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewAgentMemoryHandler(memory), participants, cfg
}

func TestAgentMemoryHandler_Set_Success(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-set@example.com", "MemorySet")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))

	req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/preferences", token, map[string]any{
		"theme": "dark",
		"lang":  "en",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]string
	testutil.DecodeResponse(t, rec, &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
	if resp["key"] != "preferences" {
		t.Errorf("expected key 'preferences', got %q", resp["key"])
	}
}

func TestAgentMemoryHandler_SetAndGet(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-setget@example.com", "MemorySetGet")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))
	mux.Handle("GET /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Get)))

	// Set
	setReq := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/context", token, map[string]any{
		"last_topic": "quantum computing",
	})
	setRec := httptest.NewRecorder()
	mux.ServeHTTP(setRec, setReq)
	testutil.AssertStatus(t, setRec, http.StatusOK)

	// Get
	getReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory/context", token, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	testutil.AssertStatus(t, getRec, http.StatusOK)

	var entry repository.AgentMemoryEntry
	testutil.DecodeResponse(t, getRec, &entry)
	if entry.Key != "context" {
		t.Errorf("expected key 'context', got %q", entry.Key)
	}

	var val map[string]any
	if err := json.Unmarshal(entry.Value, &val); err != nil {
		t.Fatalf("unmarshal value: %v", err)
	}
	if val["last_topic"] != "quantum computing" {
		t.Errorf("expected last_topic 'quantum computing', got %v", val["last_topic"])
	}
}

func TestAgentMemoryHandler_Get_NotFound(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-notfound@example.com", "MemoryNotFound")

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Get)))

	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory/nonexistent", token, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestAgentMemoryHandler_List(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-list@example.com", "MemoryList")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))
	mux.Handle("GET /api/v1/agent-memory", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List)))

	// Set a few keys
	for _, key := range []string{"config.theme", "config.lang", "data.cache"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/"+key, token, "value")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusOK)
	}

	// List all
	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory", token, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var entries []repository.AgentMemoryEntry
	testutil.DecodeResponse(t, rec, &entries)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// List with prefix
	req = testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory?prefix=config.", token, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	testutil.DecodeResponse(t, rec, &entries)
	if len(entries) != 2 {
		t.Errorf("expected 2 config entries, got %d", len(entries))
	}
}

func TestAgentMemoryHandler_Delete(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-del@example.com", "MemoryDel")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))
	mux.Handle("DELETE /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Delete)))
	mux.Handle("GET /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Get)))

	// Set a key
	req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/temp", token, "temporary")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	// Delete it
	req = testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-memory/temp", token, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	// Verify it's gone
	req = testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory/temp", token, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestAgentMemoryHandler_DeleteAll(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-delall@example.com", "MemoryDelAll")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))
	mux.Handle("DELETE /api/v1/agent-memory", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.DeleteAll)))
	mux.Handle("GET /api/v1/agent-memory", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List)))

	// Set keys
	for _, key := range []string{"a", "b", "c"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/"+key, token, 1)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusOK)
	}

	// Clear all
	req := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-memory", token, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	// Verify empty
	req = testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-memory", token, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var entries []repository.AgentMemoryEntry
	testutil.DecodeResponse(t, rec, &entries)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
}

func TestAgentMemoryHandler_Set_KeyTooLong(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-longkey@example.com", "MemoryLongKey")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))

	longKey := strings.Repeat("x", 257)
	req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/agent-memory/"+longKey, token, "value")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentMemoryHandler_Set_InvalidJSON(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-badjson@example.com", "MemoryBadJSON")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-memory/bad", strings.NewReader("not valid json {{{"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentMemoryHandler_Set_EmptyBody(t *testing.T) {
	handler, participants, cfg := setupAgentMemoryTest(t)
	_, token := registerTestUser(t, participants, cfg, "memory-empty@example.com", "MemoryEmpty")

	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/agent-memory/{key}", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Set)))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-memory/empty", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentMemoryHandler_Unauthorized(t *testing.T) {
	handler, _, _ := setupAgentMemoryTest(t)

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/agent-memory/{key}", middleware.Auth("test-secret-key-for-testing")(http.HandlerFunc(handler.Get)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-memory/test", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}
