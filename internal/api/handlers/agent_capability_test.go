package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupCapabilityTest(t *testing.T) (*handlers.AgentCapabilityHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")
	participants := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewAgentCapabilityHandler(capRepo), participants, cfg
}

func TestAgentCapabilityHandler_Register(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-reg@example.com", "CapReg")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"capability":  "research",
		"description": "Deep research on AI topics",
		"endpoint_url": "https://example.com/research",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var cap repository.AgentCapability
	testutil.DecodeResponse(t, rec, &cap)

	if cap.ID == "" {
		t.Error("expected non-empty capability ID")
	}
	if cap.Capability != "research" {
		t.Errorf("expected capability 'research', got %q", cap.Capability)
	}
	if cap.Description != "Deep research on AI topics" {
		t.Errorf("expected description, got %q", cap.Description)
	}
}

func TestAgentCapabilityHandler_Register_MissingCapability(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-miss@example.com", "CapMiss")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"description": "No capability name provided",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentCapabilityHandler_Register_InvalidEndpointURL(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-badurl@example.com", "BadURL")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"capability":   "research",
		"endpoint_url": "ftp://not-http.com",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentCapabilityHandler_Unregister(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-unreg@example.com", "CapUnreg")

	// Register a capability first
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"capability": "research",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	// Unregister it
	delReq := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-capabilities/research", token, nil)
	delReq.SetPathValue("capability", "research")
	delRec := httptest.NewRecorder()
	delProtected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Unregister))
	delProtected.ServeHTTP(delRec, delReq)

	testutil.AssertStatus(t, delRec, http.StatusOK)
}

func TestAgentCapabilityHandler_Unregister_NotFound(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-unreg-nf@example.com", "CapUnregNF")

	req := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-capabilities/nonexistent", token, nil)
	req.SetPathValue("capability", "nonexistent")
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Unregister))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestAgentCapabilityHandler_ListMine(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-list@example.com", "CapList")

	// Register two capabilities
	for _, cap := range []string{"research", "translation"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
			"capability": cap,
		})
		rec := httptest.NewRecorder()
		protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
		protected.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusCreated)
	}

	// List
	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-capabilities", token, nil)
	listRec := httptest.NewRecorder()
	listProtected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.ListMine))
	listProtected.ServeHTTP(listRec, listReq)

	testutil.AssertStatus(t, listRec, http.StatusOK)

	var caps []repository.AgentCapability
	testutil.DecodeResponse(t, listRec, &caps)

	if len(caps) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(caps))
	}
}

func TestAgentCapabilityHandler_ListMine_Empty(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-empty@example.com", "CapEmpty")

	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-capabilities", token, nil)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.ListMine))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var caps []repository.AgentCapability
	testutil.DecodeResponse(t, rec, &caps)

	if len(caps) != 0 {
		t.Errorf("expected empty list, got %d items", len(caps))
	}
}

func TestAgentCapabilityHandler_Search(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-search@example.com", "CapSearch")

	// Register a capability
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"capability":  "research",
		"description": "Research capabilities",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	// Search for it (public endpoint)
	searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/discover?capability=research", nil)
	searchRec := httptest.NewRecorder()
	handler.Search(searchRec, searchReq)

	testutil.AssertStatus(t, searchRec, http.StatusOK)

	var result struct {
		Agents []repository.AgentCapabilityWithAgent `json:"agents"`
		Total  int                                   `json:"total"`
	}
	testutil.DecodeResponse(t, searchRec, &result)

	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
	if len(result.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(result.Agents))
	}
}

func TestAgentCapabilityHandler_SearchByCapability(t *testing.T) {
	handler, participants, cfg := setupCapabilityTest(t)
	_, token := registerTestUser(t, participants, cfg, "cap-searchby@example.com", "CapSearchBy")

	// Register a capability
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-capabilities", token, map[string]any{
		"capability": "translation",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	// Search by capability path param
	searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/discover/translation", nil)
	searchReq.SetPathValue("capability", "translation")
	searchRec := httptest.NewRecorder()
	handler.SearchByCapability(searchRec, searchReq)

	testutil.AssertStatus(t, searchRec, http.StatusOK)

	var result struct {
		Agents []repository.AgentCapabilityWithAgent `json:"agents"`
		Total  int                                   `json:"total"`
	}
	testutil.DecodeResponse(t, searchRec, &result)

	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
}

func TestAgentCapabilityHandler_Unauthenticated(t *testing.T) {
	handler, _, _ := setupCapabilityTest(t)

	// Try to register without auth
	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/agent-capabilities", map[string]any{
		"capability": "research",
	})
	rec := httptest.NewRecorder()
	handler.Register(rec, req)
	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}
