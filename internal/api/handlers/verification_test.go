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
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupVerificationTest(t *testing.T) (*handlers.VerificationHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "human_verifications", "epistemic_votes", "provenances", "reputation_events", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	verifications := repository.NewVerificationRepo(pool)
	reputation := repository.NewReputationRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewVerificationHandler(verifications, posts, reputation), participants, communities, posts, cfg
}

func TestVerificationHandler_Verify_Success(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVerificationTest(t)
	participant, token := registerTestUser(t, participants, cfg, "verifier@example.com", "Verifier")
	community := createTestCommunity(t, communities, participant.ID, "verify-test")
	// Agent post -- we use the same human as author for simplicity (AuthorType overridden)
	post := createTestPost(t, posts, community.ID, participant.ID)
	// Override author_type to agent for the test
	pool := database.TestPool(t)
	_, err := pool.Exec(context.Background(), `UPDATE posts SET author_type = 'agent' WHERE id = $1`, post.ID)
	if err != nil {
		t.Fatalf("updating post author_type: %v", err)
	}

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req.SetPathValue("id", post.ID)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Verify))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]string
	testutil.DecodeResponse(t, rec, &resp)

	if resp["status"] != "verified" {
		t.Errorf("expected status 'verified', got %q", resp["status"])
	}
}

func TestVerificationHandler_Verify_HumanPostRejected(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVerificationTest(t)
	participant, token := registerTestUser(t, participants, cfg, "verifier-human@example.com", "VerifierHuman")
	community := createTestCommunity(t, communities, participant.ID, "verify-human-test")
	// This is a human-authored post -- should be rejected
	post := createTestPost(t, posts, community.ID, participant.ID)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req.SetPathValue("id", post.ID)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Verify))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestVerificationHandler_Verify_Unauthenticated(t *testing.T) {
	handler, _, _, _, _ := setupVerificationTest(t)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/posts/some-id/verify", nil)
	req.SetPathValue("id", "some-id")
	rec := httptest.NewRecorder()

	handler.Verify(rec, req)

	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}

func TestVerificationHandler_Unverify(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVerificationTest(t)
	participant, token := registerTestUser(t, participants, cfg, "unverifier@example.com", "Unverifier")
	community := createTestCommunity(t, communities, participant.ID, "unverify-test")
	post := createTestPost(t, posts, community.ID, participant.ID)
	// Override author_type to agent
	pool := database.TestPool(t)
	_, err := pool.Exec(context.Background(), `UPDATE posts SET author_type = 'agent' WHERE id = $1`, post.ID)
	if err != nil {
		t.Fatalf("updating post author_type: %v", err)
	}

	protected := middleware.Auth(cfg.JWT.Secret)

	// First verify
	req1 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req1.SetPathValue("id", post.ID)
	rec1 := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Verify)).ServeHTTP(rec1, req1)
	testutil.AssertStatus(t, rec1, http.StatusOK)

	// Then unverify
	req2 := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req2.SetPathValue("id", post.ID)
	rec2 := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Unverify)).ServeHTTP(rec2, req2)
	testutil.AssertStatus(t, rec2, http.StatusOK)

	var resp map[string]string
	testutil.DecodeResponse(t, rec2, &resp)
	if resp["status"] != "unverified" {
		t.Errorf("expected status 'unverified', got %q", resp["status"])
	}
}

func TestVerificationHandler_GetStatus(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVerificationTest(t)
	participant, token := registerTestUser(t, participants, cfg, "status-checker@example.com", "StatusChecker")
	community := createTestCommunity(t, communities, participant.ID, "status-test")
	post := createTestPost(t, posts, community.ID, participant.ID)
	// Override author_type to agent
	pool := database.TestPool(t)
	_, err := pool.Exec(context.Background(), `UPDATE posts SET author_type = 'agent' WHERE id = $1`, post.ID)
	if err != nil {
		t.Fatalf("updating post author_type: %v", err)
	}

	// Verify first
	protected := middleware.Auth(cfg.JWT.Secret)
	req1 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req1.SetPathValue("id", post.ID)
	rec1 := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Verify)).ServeHTTP(rec1, req1)
	testutil.AssertStatus(t, rec1, http.StatusOK)

	// Get status (unauthenticated)
	req2 := testutil.JSONRequest(t, http.MethodGet, "/api/v1/posts/"+post.ID+"/verify", nil)
	req2.SetPathValue("id", post.ID)
	rec2 := httptest.NewRecorder()
	handler.GetStatus(rec2, req2)
	testutil.AssertStatus(t, rec2, http.StatusOK)

	var resp map[string]any
	testutil.DecodeResponse(t, rec2, &resp)

	if resp["count"].(float64) != 1 {
		t.Errorf("expected count 1, got %v", resp["count"])
	}
	// Unauthenticated, verified should be false
	if resp["verified"].(bool) {
		t.Error("expected verified false for unauthenticated, got true")
	}

	// Get status (authenticated)
	req3 := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/posts/"+post.ID+"/verify", token, nil)
	req3.SetPathValue("id", post.ID)
	rec3 := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.GetStatus)).ServeHTTP(rec3, req3)
	testutil.AssertStatus(t, rec3, http.StatusOK)

	var resp3 map[string]any
	testutil.DecodeResponse(t, rec3, &resp3)

	if !resp3["verified"].(bool) {
		t.Error("expected verified true for authenticated verifier, got false")
	}
}
