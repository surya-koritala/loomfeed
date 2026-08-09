package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupEpistemicTest(t *testing.T) (*handlers.EpistemicHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "provenances", "reputation_events", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	epistemic := repository.NewEpistemicRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewEpistemicHandler(epistemic), participants, communities, posts, cfg
}

func TestEpistemicHandler_Vote(t *testing.T) {
	handler, participants, communities, posts, cfg := setupEpistemicTest(t)
	participant, token := registerTestUser(t, participants, cfg, "epistemic-voter@example.com", "EpistemicVoter")
	community := createTestCommunity(t, communities, participant.ID, "epistemic-vote-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/epistemic", token, map[string]string{
		"status": "supported",
	})
	req.SetPathValue("id", post.ID)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Vote))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var result repository.EpistemicResult
	testutil.DecodeResponse(t, rec, &result)

	if result.Status != "supported" {
		t.Errorf("expected status 'supported', got %q", result.Status)
	}
	if result.TotalVotes != 1 {
		t.Errorf("expected 1 total vote, got %d", result.TotalVotes)
	}
	if result.UserVote != "supported" {
		t.Errorf("expected user_vote 'supported', got %q", result.UserVote)
	}
}

func TestEpistemicHandler_Vote_InvalidStatus(t *testing.T) {
	handler, participants, communities, posts, cfg := setupEpistemicTest(t)
	participant, token := registerTestUser(t, participants, cfg, "epistemic-invalid@example.com", "EpistemicInvalid")
	community := createTestCommunity(t, communities, participant.ID, "epistemic-invalid-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/epistemic", token, map[string]string{
		"status": "maybe",
	})
	req.SetPathValue("id", post.ID)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Vote))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestEpistemicHandler_Vote_Unauthenticated(t *testing.T) {
	handler, _, _, _, _ := setupEpistemicTest(t)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/posts/some-id/epistemic", map[string]string{
		"status": "supported",
	})
	req.SetPathValue("id", "some-id")
	rec := httptest.NewRecorder()

	handler.Vote(rec, req)

	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}

func TestEpistemicHandler_Get(t *testing.T) {
	handler, participants, communities, posts, cfg := setupEpistemicTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "epistemic-getter@example.com", "EpistemicGetter")
	community := createTestCommunity(t, communities, participant.ID, "epistemic-get-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	// First cast a vote directly via the repository
	pool := database.TestPool(t)
	eRepo := repository.NewEpistemicRepo(pool)
	if err := eRepo.Vote(context.Background(), post.ID, participant.ID, "contested"); err != nil {
		t.Fatalf("Vote: %v", err)
	}

	// GET without auth
	req := testutil.JSONRequest(t, http.MethodGet, "/api/v1/posts/"+post.ID+"/epistemic", nil)
	req.SetPathValue("id", post.ID)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var result repository.EpistemicResult
	testutil.DecodeResponse(t, rec, &result)

	if result.Status != "contested" {
		t.Errorf("expected status 'contested', got %q", result.Status)
	}
	if result.TotalVotes != 1 {
		t.Errorf("expected 1 total vote, got %d", result.TotalVotes)
	}
	// No auth -> no user_vote
	if result.UserVote != "" {
		t.Errorf("expected empty user_vote for unauthenticated request, got %q", result.UserVote)
	}
}

func TestEpistemicHandler_Get_NotFound(t *testing.T) {
	handler, _, _, _, _ := setupEpistemicTest(t)

	req := testutil.JSONRequest(t, http.MethodGet, "/api/v1/posts/00000000-0000-0000-0000-000000000000/epistemic", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestEpistemicHandler_Vote_ChangeVote(t *testing.T) {
	handler, participants, communities, posts, cfg := setupEpistemicTest(t)
	participant, token := registerTestUser(t, participants, cfg, "epistemic-changer@example.com", "EpistemicChanger")
	community := createTestCommunity(t, communities, participant.ID, "epistemic-change-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Vote))

	// Vote supported
	req1 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/epistemic", token, map[string]string{
		"status": "supported",
	})
	req1.SetPathValue("id", post.ID)
	rec1 := httptest.NewRecorder()
	protected.ServeHTTP(rec1, req1)
	testutil.AssertStatus(t, rec1, http.StatusOK)

	// Change to refuted
	req2 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/epistemic", token, map[string]string{
		"status": "refuted",
	})
	req2.SetPathValue("id", post.ID)
	rec2 := httptest.NewRecorder()
	protected.ServeHTTP(rec2, req2)
	testutil.AssertStatus(t, rec2, http.StatusOK)

	var result repository.EpistemicResult
	testutil.DecodeResponse(t, rec2, &result)

	if result.Status != "refuted" {
		t.Errorf("expected status 'refuted' after change, got %q", result.Status)
	}
	if result.TotalVotes != 1 {
		t.Errorf("expected 1 total vote after change, got %d", result.TotalVotes)
	}
	if result.UserVote != "refuted" {
		t.Errorf("expected user_vote 'refuted', got %q", result.UserVote)
	}
}
