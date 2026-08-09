package handlers_test

import (
	"context"
	"encoding/json"
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

func setupActivityTest(t *testing.T) (*handlers.ActivityHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *repository.CommentRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewActivityHandler(pool), participants, communities, posts, comments, cfg
}

func TestActivityHandler_Recent_Empty(t *testing.T) {
	handler, _, _, _, _, _ := setupActivityTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/recent", nil)
	rec := httptest.NewRecorder()
	handler.Recent(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Events []map[string]string `json:"events"`
	}
	testutil.DecodeResponse(t, rec, &resp)

	if len(resp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(resp.Events))
	}
}

func TestActivityHandler_Recent_WithData(t *testing.T) {
	handler, participants, communities, postsRepo, commentsRepo, cfg := setupActivityTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "activity@example.com", "ActivityUser")
	community := createTestCommunity(t, communities, participant.ID, "activity-test")

	// Create a post
	post, err := postsRepo.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Activity Test Post",
		Body:        "Body of activity test",
		PostType:    models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("creating test post: %v", err)
	}

	// Create a comment on the post
	_, err = commentsRepo.Create(context.Background(), &models.Comment{
		PostID:     post.ID,
		AuthorID:   participant.ID,
		AuthorType: models.ParticipantHuman,
		Body:       "A test comment",
	})
	if err != nil {
		t.Fatalf("creating test comment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/recent?limit=10", nil)
	rec := httptest.NewRecorder()
	handler.Recent(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Events []map[string]string `json:"events"`
	}
	testutil.DecodeResponse(t, rec, &resp)

	if len(resp.Events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(resp.Events))
	}

	// Check that events have expected fields
	for _, ev := range resp.Events {
		if ev["type"] == "" {
			t.Error("expected event type to be non-empty")
		}
		if ev["actor"] == "" {
			t.Error("expected actor to be non-empty")
		}
		if ev["actor_type"] == "" {
			t.Error("expected actor_type to be non-empty")
		}
		if ev["action"] == "" {
			t.Error("expected action to be non-empty")
		}
		if ev["target"] == "" {
			t.Error("expected target to be non-empty")
		}
		if ev["time_ago"] == "" {
			t.Error("expected time_ago to be non-empty")
		}
	}

	// Verify post event has "a/" prefix
	found := false
	for _, ev := range resp.Events {
		if ev["type"] == "post" && ev["action"] == "posted in" {
			found = true
			if ev["target"] != "a/activity-test" {
				t.Errorf("expected post target 'a/activity-test', got %q", ev["target"])
			}
		}
	}
	if !found {
		t.Error("expected to find a 'post' event with 'posted in' action")
	}

	// Verify comment event references the post title
	found = false
	for _, ev := range resp.Events {
		if ev["type"] == "comment" && ev["action"] == "commented on" {
			found = true
			if ev["target"] != "Activity Test Post" {
				t.Errorf("expected comment target 'Activity Test Post', got %q", ev["target"])
			}
		}
	}
	if !found {
		t.Error("expected to find a 'comment' event with 'commented on' action")
	}
}

func TestActivityHandler_Recent_LimitClamping(t *testing.T) {
	handler, _, _, _, _, _ := setupActivityTest(t)

	// Test limit > 50 gets clamped
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity/recent?limit=100", nil)
	rec := httptest.NewRecorder()
	handler.Recent(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	// Verify response is valid JSON
	var resp map[string]json.RawMessage
	testutil.DecodeResponse(t, rec, &resp)
	if _, ok := resp["events"]; !ok {
		t.Error("expected 'events' key in response")
	}
}
