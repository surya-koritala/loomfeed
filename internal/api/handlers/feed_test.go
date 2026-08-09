package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupFeedTest(t *testing.T) (*handlers.FeedHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	posts := repository.NewPostRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewFeedHandler(posts, communities, cfg), participants, communities, posts, cfg
}

// seedPosts creates n posts in the given community.
func seedPosts(t *testing.T, posts *repository.PostRepo, communityID, authorID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := posts.Create(context.Background(), &models.Post{
			CommunityID: communityID,
			AuthorID:    authorID,
			AuthorType:  models.ParticipantHuman,
			Title:       "Feed Post",
			Body:        "Feed post body",
			PostType: models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("creating seed post %d: %v", i, err)
		}
	}
}

func TestFeedHandler_Global_Success(t *testing.T) {
	handler, participants, communities, postsRepo, cfg := setupFeedTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "feed-global@example.com", "Feed Global")
	community := createTestCommunity(t, communities, participant.ID, "feed-global-test")
	seedPosts(t, postsRepo, community.ID, participant.ID, 3)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed?sort=new&limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	handler.Global(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp models.PaginatedResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Total < 3 {
		t.Errorf("expected total >= 3, got %d", resp.Total)
	}
	if resp.Limit != 10 {
		t.Errorf("expected limit 10, got %d", resp.Limit)
	}
}

func TestFeedHandler_ByCommunity_Success(t *testing.T) {
	handler, participants, communities, postsRepo, cfg := setupFeedTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "feed-community@example.com", "Feed Community")
	community := createTestCommunity(t, communities, participant.ID, "feed-comm-test")
	seedPosts(t, postsRepo, community.ID, participant.ID, 2)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/communities/{slug}/feed", handler.ByCommunity)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities/feed-comm-test/feed?sort=new&limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp models.PaginatedResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Total < 2 {
		t.Errorf("expected total >= 2, got %d", resp.Total)
	}
}

func TestFeedHandler_ByCommunity_NotFound(t *testing.T) {
	handler, _, _, _, _ := setupFeedTest(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/communities/{slug}/feed", handler.ByCommunity)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities/nonexistent-slug/feed", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}
