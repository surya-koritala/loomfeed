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
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupCommentTest(t *testing.T) (*handlers.CommentHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	comments := repository.NewCommentRepo(pool)
	provenances := repository.NewProvenanceRepo(pool)
	notifications := repository.NewNotificationRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	h := handlers.NewCommentHandler(comments, provenances, notifications, cfg)
	h.WithPosts(posts)
	return h, participants, communities, posts, cfg
}

// createTestPost creates a post for testing and returns it.
func createTestPost(t *testing.T, posts *repository.PostRepo, communityID, authorID string) *models.Post {
	t.Helper()
	post, err := posts.Create(context.Background(), &models.Post{
		CommunityID: communityID,
		AuthorID:    authorID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Test Post",
		Body:        "Test post body",
		PostType: models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("creating test post: %v", err)
	}
	return post
}

func TestCommentHandler_Create_Success(t *testing.T) {
	handler, participants, communities, posts, cfg := setupCommentTest(t)
	participant, token := registerTestUser(t, participants, cfg, "commenter@example.com", "Commenter")
	community := createTestCommunity(t, communities, participant.ID, "comment-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	// Use a ServeMux to extract the path value.
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/comments", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)))

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/comments", token, models.CreateCommentRequest{
		Body: "Great post!",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var comment models.Comment
	testutil.DecodeResponse(t, rec, &comment)

	if comment.ID == "" {
		t.Error("expected non-empty comment ID")
	}
	if comment.Body != "Great post!" {
		t.Errorf("expected body 'Great post!', got %q", comment.Body)
	}
	if comment.PostID != post.ID {
		t.Errorf("expected post_id %q, got %q", post.ID, comment.PostID)
	}
}

func TestCommentHandler_Create_MissingBody(t *testing.T) {
	handler, participants, communities, posts, cfg := setupCommentTest(t)
	participant, token := registerTestUser(t, participants, cfg, "nobody@example.com", "NoBody")
	community := createTestCommunity(t, communities, participant.ID, "comment-missing-body")
	post := createTestPost(t, posts, community.ID, participant.ID)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/comments", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)))

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/comments", token, models.CreateCommentRequest{
		Body: "",
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestCommentHandler_ListByPost_Success(t *testing.T) {
	handler, participants, communities, posts, cfg := setupCommentTest(t)
	participant, token := registerTestUser(t, participants, cfg, "list-comments@example.com", "ListComments")
	community := createTestCommunity(t, communities, participant.ID, "list-comments-test")
	post := createTestPost(t, posts, community.ID, participant.ID)

	// Create two comments.
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/comments", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)))
	mux.HandleFunc("GET /api/v1/posts/{id}/comments", handler.ListByPost)

	for _, body := range []string{"Comment one", "Comment two"} {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/comments", token, models.CreateCommentRequest{
			Body: body,
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusCreated)
	}

	// List comments.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+post.ID+"/comments?limit=10&offset=0", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)

	testutil.AssertStatus(t, listRec, http.StatusOK)

	var comments []models.CommentWithAuthor
	testutil.DecodeResponse(t, listRec, &comments)

	if len(comments) < 2 {
		t.Errorf("expected at least 2 comments, got %d", len(comments))
	}
}
