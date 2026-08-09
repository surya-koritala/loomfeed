package handlers_test

import (
	"context"
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

func setupAgentSubTest(t *testing.T) (*handlers.AgentSubscriptionHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")
	participants := repository.NewParticipantRepo(pool)
	agentSubs := repository.NewAgentSubscriptionRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewAgentSubscriptionHandler(agentSubs), participants, cfg
}

func TestAgentSubscriptionHandler_Create(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	participant, token := registerTestUser(t, participants, cfg, "agsub@example.com", "SubTester")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
		"subscription_type": "community",
		"filter_value":      "osai",
		"webhook_url":       "https://my-agent.example.com/hook",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var sub repository.AgentSubscription
	testutil.DecodeResponse(t, rec, &sub)

	if sub.ID == "" {
		t.Error("expected non-empty subscription ID")
	}
	if sub.AgentID != participant.ID {
		t.Errorf("expected agent_id %q, got %q", participant.ID, sub.AgentID)
	}
	if sub.SubscriptionType != "community" {
		t.Errorf("expected subscription_type 'community', got %q", sub.SubscriptionType)
	}
	if sub.FilterValue != "osai" {
		t.Errorf("expected filter_value 'osai', got %q", sub.FilterValue)
	}
}

func TestAgentSubscriptionHandler_Create_InvalidType(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-bad@example.com", "BadType")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
		"subscription_type": "invalid_type",
		"filter_value":      "osai",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentSubscriptionHandler_Create_MissingFields(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-miss@example.com", "MissingFields")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
		"subscription_type": "community",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAgentSubscriptionHandler_List(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-list@example.com", "Lister")

	// Create two subscriptions first
	for _, fv := range []string{"osai", "general"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
			"subscription_type": "community",
			"filter_value":      fv,
		})
		rec := httptest.NewRecorder()
		protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
		protected.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusCreated)
	}

	// List
	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-subscriptions", token, nil)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var subs []repository.AgentSubscription
	testutil.DecodeResponse(t, rec, &subs)

	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(subs))
	}
}

func TestAgentSubscriptionHandler_Delete(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-del@example.com", "Deleter")

	// Create a subscription
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
		"subscription_type": "keyword",
		"filter_value":      "golang",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	var sub repository.AgentSubscription
	testutil.DecodeResponse(t, createRec, &sub)

	// Delete it
	delReq := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-subscriptions/"+sub.ID, token, nil)
	delReq.SetPathValue("id", sub.ID)
	delRec := httptest.NewRecorder()

	delProtected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Delete))
	delProtected.ServeHTTP(delRec, delReq)

	testutil.AssertStatus(t, delRec, http.StatusOK)

	// Verify list is empty
	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-subscriptions", token, nil)
	listRec := httptest.NewRecorder()
	listProtected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List))
	listProtected.ServeHTTP(listRec, listReq)

	var subs []repository.AgentSubscription
	testutil.DecodeResponse(t, listRec, &subs)

	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after delete, got %d", len(subs))
	}
}

func TestAgentSubscriptionHandler_Unauthenticated(t *testing.T) {
	handler, _, _ := setupAgentSubTest(t)

	// Try to create without auth
	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/agent-subscriptions", map[string]any{
		"subscription_type": "community",
		"filter_value":      "osai",
	})
	rec := httptest.NewRecorder()

	// Call directly without auth middleware — claims will be nil
	handler.Create(rec, req)
	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}

func TestAgentSubscriptionHandler_ListEmpty(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-empty@example.com", "EmptyList")

	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-subscriptions", token, nil)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var subs []repository.AgentSubscription
	testutil.DecodeResponse(t, rec, &subs)

	if len(subs) != 0 {
		t.Errorf("expected empty list, got %d items", len(subs))
	}
}

func TestAgentSubscriptionHandler_Delete_NotFound(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-delnf@example.com", "DelNotFound")

	req := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/agent-subscriptions/00000000-0000-0000-0000-000000000000", token, nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Delete))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestAgentSubscriptionHandler_Create_AllTypes(t *testing.T) {
	handler, participants, cfg := setupAgentSubTest(t)
	_, token := registerTestUser(t, participants, cfg, "agsub-types@example.com", "AllTypes")

	types := []struct {
		subType     string
		filterValue string
	}{
		{"community", "osai"},
		{"keyword", "golang"},
		{"mention", "agent-123"},
		{"post_type", "debate"},
	}

	for _, tc := range types {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/agent-subscriptions", token, map[string]any{
			"subscription_type": tc.subType,
			"filter_value":      tc.filterValue,
		})
		rec := httptest.NewRecorder()
		protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
		protected.ServeHTTP(rec, req)

		testutil.AssertStatus(t, rec, http.StatusCreated)
	}

	// Verify all 4 were created
	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/agent-subscriptions", token, nil)
	listRec := httptest.NewRecorder()
	listProtected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.List))
	listProtected.ServeHTTP(listRec, listReq)

	var subs []repository.AgentSubscription
	testutil.DecodeResponse(t, listRec, &subs)

	if len(subs) != 4 {
		t.Errorf("expected 4 subscriptions, got %d", len(subs))
	}
}

// Ensure the notifier function requires context (smoke test).
func TestNotifySubscribers_NoSubs_NoPanic(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")
	subRepo := repository.NewAgentSubscriptionRepo(pool)

	pRepo := repository.NewParticipantRepo(pool)
	ctx := context.Background()
	owner, _ := registerTestUser(t, pRepo, &config.Config{
		JWT: config.JWTConfig{Secret: "test", Expiry: time.Hour},
	}, "notify-test@example.com", "NotifyTester")

	cRepo := repository.NewCommunityRepo(pool)
	community, err := cRepo.Create(ctx, &models.Community{
		Name:      "Notify Test",
		Slug:      "notify-test",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("creating community: %v", err)
	}

	postRepo := repository.NewPostRepo(pool)
	post, err := postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Test Post",
		Body:        "This is a test",
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}

	// This should not panic even with no subscriptions
	handlers.NotifySubscribers(subRepo, post, "notify-test", "NotifyTester")
	// Give goroutine time to complete
	time.Sleep(100 * time.Millisecond)
}
