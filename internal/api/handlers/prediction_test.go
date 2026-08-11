package handlers_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func TestPredictionHandler_PostLifecycleTriggersScorecardAndAccuracy(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"scorecard_history", "agent_scorecards", "prediction_stats", "predictions",
		"votes", "posts", "communities", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	predictions := repository.NewPredictionRepo(pool)
	author := createPredictionTestHuman(t, ctx, participants, "prediction-handler-author")
	community, err := communities.Create(ctx, &models.Community{
		Name: "Prediction Handler", Slug: "prediction-handler", CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman,
		Title: "A prediction", Body: "This post makes a falsifiable prediction.",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	hub := events.NewHub()
	triggered := hub.Subscribe("__scorecard_worker__")
	t.Cleanup(func() { hub.Unsubscribe("__scorecard_worker__", triggered) })
	handler := handlers.NewPredictionHandler(predictions)
	handler.WithScorecardTrigger(hub)

	createReq := predictionAuthedRequest(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/predictions", map[string]any{
		"subject":           "The launch succeeds",
		"predicted_outcome": "success",
		"confidence":        0.8,
		"resolve_by":        time.Now().UTC().Add(time.Hour),
		"reasoning":         "All preflight checks passed.",
	}, author)
	createReq.SetPathValue("id", post.ID)
	createRec := httptest.NewRecorder()
	handler.UpsertPost(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data models.Prediction `json:"data"`
	}
	testutil.DecodeResponse(t, createRec, &created)
	if created.Data.ID == "" || created.Data.Subject != "The launch succeeds" {
		t.Fatalf("created prediction = %#v", created.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+post.ID+"/predictions", nil)
	listReq.SetPathValue("id", post.ID)
	listRec := httptest.NewRecorder()
	handler.ListPost(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Data []models.Prediction `json:"data"`
	}
	testutil.DecodeResponse(t, listRec, &listed)
	if len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("listed predictions = %#v", listed.Data)
	}

	earlyReq := predictionAuthedRequest(t, http.MethodPost, "/api/v1/predictions/"+created.Data.ID+"/resolve", map[string]string{
		"resolution": "success",
	}, author)
	earlyReq.SetPathValue("id", created.Data.ID)
	earlyRec := httptest.NewRecorder()
	handler.Resolve(earlyRec, earlyReq)
	if earlyRec.Code != http.StatusConflict {
		t.Fatalf("early resolve status=%d body=%s", earlyRec.Code, earlyRec.Body.String())
	}

	if _, err := pool.Exec(ctx, `UPDATE predictions SET resolve_by = NOW() - INTERVAL '1 minute' WHERE id = $1`, created.Data.ID); err != nil {
		t.Fatalf("make prediction resolvable: %v", err)
	}
	resolveReq := predictionAuthedRequest(t, http.MethodPost, "/api/v1/predictions/"+created.Data.ID+"/resolve", map[string]string{
		"resolution": "SUCCESS",
	}, author)
	resolveReq.SetPathValue("id", created.Data.ID)
	resolveRec := httptest.NewRecorder()
	handler.Resolve(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", resolveRec.Code, resolveRec.Body.String())
	}

	select {
	case event := <-triggered:
		var payload struct {
			ParticipantID string `json:"participant_id"`
		}
		if event.Type != "scorecard.trigger" || json.Unmarshal([]byte(event.Data), &payload) != nil || payload.ParticipantID != author.ID {
			t.Fatalf("scorecard event = %#v payload = %#v", event, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scorecard trigger")
	}

	accuracyReq := httptest.NewRequest(http.MethodGet, "/api/v1/scorecard/"+author.ID+"/accuracy", nil)
	accuracyReq.SetPathValue("id", author.ID)
	accuracyRec := httptest.NewRecorder()
	handlers.NewAccuracyHandler(pool).Get(accuracyRec, accuracyReq)
	if accuracyRec.Code != http.StatusOK {
		t.Fatalf("accuracy status=%d body=%s", accuracyRec.Code, accuracyRec.Body.String())
	}
	var accuracy struct {
		TotalResolved      int     `json:"total_resolved"`
		CorrectCount       int     `json:"correct_count"`
		Accuracy           float64 `json:"accuracy"`
		CalibratedAccuracy float64 `json:"calibrated_accuracy"`
	}
	testutil.DecodeResponse(t, accuracyRec, &accuracy)
	if accuracy.TotalResolved != 1 || accuracy.CorrectCount != 1 || accuracy.Accuracy != 1 || math.Abs(accuracy.CalibratedAccuracy-0.96) > 1e-6 {
		t.Fatalf("accuracy response = %#v", accuracy)
	}
}

func TestPredictionHandler_ValidatesInputAndPostOwnership(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "prediction_stats", "predictions", "posts", "communities", "participants")
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	author := createPredictionTestHuman(t, ctx, participants, "prediction-validation-author")
	other := createPredictionTestHuman(t, ctx, participants, "prediction-validation-other")
	community, err := communities.Create(ctx, &models.Community{
		Name: "Prediction Validation", Slug: "prediction-validation", CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman,
		Title: "Owned post", Body: "Only its author may attach a prediction.",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	handler := handlers.NewPredictionHandler(repository.NewPredictionRepo(pool))

	tests := []struct {
		name   string
		actor  *models.Participant
		body   map[string]any
		status int
	}{
		{
			name: "confidence over one", actor: author, status: http.StatusBadRequest,
			body: map[string]any{"subject": "x", "predicted_outcome": "yes", "confidence": 1.1, "resolve_by": time.Now().Add(time.Hour)},
		},
		{
			name: "past deadline", actor: author, status: http.StatusBadRequest,
			body: map[string]any{"subject": "x", "predicted_outcome": "yes", "confidence": 0.5, "resolve_by": time.Now().Add(-time.Hour)},
		},
		{
			name: "reasoning too long", actor: author, status: http.StatusBadRequest,
			body: map[string]any{"subject": "x", "predicted_outcome": "yes", "confidence": 0.5, "resolve_by": time.Now().Add(time.Hour), "reasoning": strings.Repeat("r", 2001)},
		},
		{
			name: "not post author", actor: other, status: http.StatusForbidden,
			body: map[string]any{"subject": "x", "predicted_outcome": "yes", "confidence": 0.5, "resolve_by": time.Now().Add(time.Hour)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := predictionAuthedRequest(t, http.MethodPost, "/api/v1/posts/"+post.ID+"/predictions", tc.body, tc.actor)
			req.SetPathValue("id", post.ID)
			rec := httptest.NewRecorder()
			handler.UpsertPost(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func createPredictionTestHuman(t *testing.T, ctx context.Context, repo *repository.ParticipantRepo, suffix string) *models.Participant {
	t.Helper()
	p, err := repo.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: suffix}, Email: suffix + "@example.com",
		PasswordHash: "test-password-hash", PreferredLanguage: "en", NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("create participant: %v", err)
	}
	return p
}

func predictionAuthedRequest(t *testing.T, method, target string, body any, actor *models.Participant) *http.Request {
	t.Helper()
	req := testutil.JSONRequest(t, method, target, body)
	claims := &auth.Claims{ParticipantID: actor.ID, ParticipantType: string(actor.Type)}
	return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
}
