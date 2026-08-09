package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupCommunityTest(t *testing.T) (*handlers.CommunityHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "community_subscriptions", "communities", "human_users", "participants")
	communities := repository.NewCommunityRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewCommunityHandler(communities, cfg), participants, cfg
}

// registerTestUser creates a human user and returns the participant and a JWT token.
func registerTestUser(t *testing.T, participants *repository.ParticipantRepo, cfg *config.Config, email, displayName string) (*models.Participant, string) {
	t.Helper()
	hash, err := auth.HashPassword("testpassword")
	if err != nil {
		t.Fatalf("hashing password: %v", err)
	}

	human := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: displayName,
			TrustScore:  10,
		},
		Email:        email,
		PasswordHash: hash,
	}

	participant, err := participants.CreateHuman(context.Background(), human)
	if err != nil {
		t.Fatalf("creating test user: %v", err)
	}

	token, err := auth.GenerateToken(cfg.JWT.Secret, cfg.JWT.Expiry, participant.ID, string(participant.Type))
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}

	return participant, token
}

func TestCommunityHandler_Create_Success(t *testing.T) {
	handler, participants, cfg := setupCommunityTest(t)
	_, token := registerTestUser(t, participants, cfg, "community-creator@example.com", "Creator")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/communities", token, models.CreateCommunityRequest{
		Name:        "Test Community",
		Slug:        "test-community",
		Description: "A community for testing handler-level community creation paths in our integration suite.",
		Category:    "meta",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var community models.Community
	testutil.DecodeResponse(t, rec, &community)

	if community.ID == "" {
		t.Error("expected non-empty community ID")
	}
	if community.Name != "Test Community" {
		t.Errorf("expected name 'Test Community', got %q", community.Name)
	}
	if community.Slug != "test-community" {
		t.Errorf("expected slug 'test-community', got %q", community.Slug)
	}
}

func TestCommunityHandler_Create_MissingFields(t *testing.T) {
	handler, participants, cfg := setupCommunityTest(t)
	_, token := registerTestUser(t, participants, cfg, "missing@example.com", "Missing")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/communities", token, models.CreateCommunityRequest{
		Name: "No Slug",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestCommunityHandler_List_Success(t *testing.T) {
	handler, participants, cfg := setupCommunityTest(t)
	_, token := registerTestUser(t, participants, cfg, "lister@example.com", "Lister")

	// Create two communities first.
	for _, slug := range []string{"comm-a", "comm-b"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/communities", token, models.CreateCommunityRequest{
			Name:        "Community " + slug,
			Slug:        slug,
			Description: "A community used by TestCommunityHandler_List_Success to exercise the list endpoint with multiple rows.",
			Category:    "meta",
		})
		rec := httptest.NewRecorder()
		protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
		protected.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusCreated)
	}

	// List communities (no auth required for list).
	listReq := testutil.JSONRequest(t, http.MethodGet, "/api/v1/communities?limit=10&offset=0", nil)
	listRec := httptest.NewRecorder()
	handler.List(listRec, listReq)

	testutil.AssertStatus(t, listRec, http.StatusOK)

	var communities []models.Community
	testutil.DecodeResponse(t, listRec, &communities)

	if len(communities) < 2 {
		t.Errorf("expected at least 2 communities, got %d", len(communities))
	}
}

func TestCommunityHandler_GetBySlug_Success(t *testing.T) {
	handler, participants, cfg := setupCommunityTest(t)
	_, token := registerTestUser(t, participants, cfg, "slugger@example.com", "Slugger")

	// Create a community.
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/communities", token, models.CreateCommunityRequest{
		Name:        "Slug Test",
		Slug:        "slug-test",
		Description: "A community for testing slug-based GET endpoints in the community handler integration suite.",
		Category:    "meta",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	// Get by slug using Go 1.22+ pattern matching.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/communities/{slug}", handler.GetBySlug)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/communities/slug-test", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	testutil.AssertStatus(t, getRec, http.StatusOK)

	var community models.Community
	testutil.DecodeResponse(t, getRec, &community)

	if community.Slug != "slug-test" {
		t.Errorf("expected slug 'slug-test', got %q", community.Slug)
	}
}

func TestCommunityHandler_GetBySlug_NotFound(t *testing.T) {
	handler, _, _ := setupCommunityTest(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/communities/{slug}", handler.GetBySlug)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}
