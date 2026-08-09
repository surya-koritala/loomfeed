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

func setupVoteTest(t *testing.T) (*handlers.VoteHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "reputation_events", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	votes := repository.NewVoteRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	reputation := repository.NewReputationRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewVoteHandler(votes, posts, comments, reputation, cfg), participants, communities, posts, cfg
}

func TestVoteHandler_Cast_Upvote(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVoteTest(t)
	participant, token := registerTestUser(t, participants, cfg, "voter@example.com", "Voter")
	community := createTestCommunity(t, communities, participant.ID, "vote-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", token, models.VoteRequest{
		TargetID:   post.ID,
		TargetType: "post",
		Direction:  "up",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]int
	testutil.DecodeResponse(t, rec, &resp)

	if resp["vote_score"] != 1 {
		t.Errorf("expected vote_score 1, got %d", resp["vote_score"])
	}
}

func TestVoteHandler_Cast_ToggleOff(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVoteTest(t)
	participant, token := registerTestUser(t, participants, cfg, "toggler@example.com", "Toggler")
	community := createTestCommunity(t, communities, participant.ID, "toggle-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast))

	// First upvote.
	req1 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", token, models.VoteRequest{
		TargetID:   post.ID,
		TargetType: "post",
		Direction:  "up",
	})
	rec1 := httptest.NewRecorder()
	protected.ServeHTTP(rec1, req1)
	testutil.AssertStatus(t, rec1, http.StatusOK)

	var resp1 map[string]int
	testutil.DecodeResponse(t, rec1, &resp1)
	if resp1["vote_score"] != 1 {
		t.Errorf("expected vote_score 1 after first upvote, got %d", resp1["vote_score"])
	}

	// Second upvote with same direction = toggle off.
	req2 := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", token, models.VoteRequest{
		TargetID:   post.ID,
		TargetType: "post",
		Direction:  "up",
	})
	rec2 := httptest.NewRecorder()
	protected.ServeHTTP(rec2, req2)
	testutil.AssertStatus(t, rec2, http.StatusOK)

	var resp2 map[string]int
	testutil.DecodeResponse(t, rec2, &resp2)
	if resp2["vote_score"] != 0 {
		t.Errorf("expected vote_score 0 after toggle off, got %d", resp2["vote_score"])
	}
}

func TestVoteHandler_Cast_InvalidDirection(t *testing.T) {
	handler, participants, communities, posts, cfg := setupVoteTest(t)
	participant, token := registerTestUser(t, participants, cfg, "bad-voter@example.com", "BadVoter")
	community := createTestCommunity(t, communities, participant.ID, "bad-vote-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/votes", token, models.VoteRequest{
		TargetID:   post.ID,
		TargetType: "post",
		Direction:  "sideways",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Cast))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}
