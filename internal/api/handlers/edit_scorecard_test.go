package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func TestEditHandler_RetractTriggersAuthorScorecard(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "votes", "posts", "communities", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	author, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Correction Author"},
		Email:             "edit-correction-author@example.com",
		PasswordHash:      "test-password-hash",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("create author: %v", err)
	}
	community, err := communities.Create(ctx, &models.Community{
		Name:      "Edit Correction",
		Slug:      "edit-correction",
		CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    author.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Correction trigger",
		Body:        "This post will be retracted.",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	hub := events.NewHub()
	triggered := hub.Subscribe("__scorecard_worker__")
	t.Cleanup(func() { hub.Unsubscribe("__scorecard_worker__", triggered) })
	handler := handlers.NewEditHandler(posts, nil, nil, &config.Config{})
	handler.WithScorecardTrigger(hub)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/retract", map[string]string{
		"notice": "The evidence did not support this claim.",
	})
	req.SetPathValue("id", post.ID)
	claims := &auth.Claims{ParticipantID: author.ID, ParticipantType: string(models.ParticipantHuman)}
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
	rec := httptest.NewRecorder()

	handler.RetractPost(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("retract status = %d, body = %s", rec.Code, rec.Body.String())
	}

	select {
	case event := <-triggered:
		if event.Type != "scorecard.trigger" {
			t.Fatalf("event type = %q, want scorecard.trigger", event.Type)
		}
		var payload struct {
			ParticipantID string `json:"participant_id"`
		}
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			t.Fatalf("decode scorecard trigger: %v", err)
		}
		if payload.ParticipantID != author.ID {
			t.Fatalf("scorecard participant = %q, want %q", payload.ParticipantID, author.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scorecard trigger")
	}
}
