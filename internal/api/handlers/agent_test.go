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
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupAgentTest(t *testing.T) (*handlers.AgentHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	participants := repository.NewParticipantRepo(pool)
	apikeys := repository.NewAPIKeyRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewAgentHandler(participants, apikeys, cfg), participants, cfg
}

func TestAgentHandler_Register_Success(t *testing.T) {
	handler, participants, cfg := setupAgentTest(t)
	_, token := registerTestUser(t, participants, cfg, "agent-owner@example.com", "Agent Owner")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agents", token, models.RegisterAgentRequest{
		DisplayName:   "MyBot",
		ModelProvider: "openai",
		ModelName:     "gpt-4",
		ProtocolType:  models.ProtocolREST,
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var resp models.RegisterAgentResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Agent == nil {
		t.Fatal("expected agent in response")
	}
	if resp.Agent.DisplayName != "MyBot" {
		t.Errorf("expected display_name 'MyBot', got %q", resp.Agent.DisplayName)
	}
	if resp.APIKey == "" {
		t.Error("expected non-empty API key")
	}
	if len(resp.APIKey) < 10 {
		t.Errorf("expected API key to be substantial, got %q", resp.APIKey)
	}
}

func TestAgentHandler_Register_MissingFields(t *testing.T) {
	handler, participants, cfg := setupAgentTest(t)
	_, token := registerTestUser(t, participants, cfg, "missing-agent@example.com", "Missing Agent")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agents", token, models.RegisterAgentRequest{
		DisplayName: "Incomplete",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Register))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentHandler_ListMine_Success(t *testing.T) {
	handler, participants, cfg := setupAgentTest(t)
	_, token := registerTestUser(t, participants, cfg, "list-agents@example.com", "List Agents")

	protected := middleware.Auth(cfg.JWT.Secret)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agents", token, models.RegisterAgentRequest{
		DisplayName:   "Bot-A",
		ModelProvider: "openai",
		ModelName:     "gpt-4",
		ProtocolType:  models.ProtocolREST,
	})
	rec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Register)).ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusCreated)

	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agents", token, nil)
	listRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.ListMine)).ServeHTTP(listRec, listReq)

	testutil.AssertStatus(t, listRec, http.StatusOK)

	var agents []models.AgentIdentity
	testutil.DecodeResponse(t, listRec, &agents)

	if len(agents) != 1 {
		t.Errorf("expected exactly 1 agent, got %d", len(agents))
	}
}

func TestAgentHandler_Register_RejectsSecondAgent(t *testing.T) {
	handler, participants, cfg := setupAgentTest(t)
	_, token := registerTestUser(t, participants, cfg, "one-agent@example.com", "One Agent Only")

	protected := middleware.Auth(cfg.JWT.Secret)

	first := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agents", token, models.RegisterAgentRequest{
		DisplayName:   "FirstBot",
		ModelProvider: "openai",
		ModelName:     "gpt-4",
		ProtocolType:  models.ProtocolREST,
	})
	firstRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Register)).ServeHTTP(firstRec, first)
	testutil.AssertStatus(t, firstRec, http.StatusCreated)

	second := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agents", token, models.RegisterAgentRequest{
		DisplayName:   "SecondBot",
		ModelProvider: "openai",
		ModelName:     "gpt-4",
		ProtocolType:  models.ProtocolREST,
	})
	secondRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Register)).ServeHTTP(secondRec, second)
	testutil.AssertStatus(t, secondRec, http.StatusConflict)
}
