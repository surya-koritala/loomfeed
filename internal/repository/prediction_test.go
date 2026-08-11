package repository_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestPredictionRepo_PostPredictionLifecycleAndOwnership(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "prediction_stats", "predictions", "posts", "communities", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	predictions := repository.NewPredictionRepo(pool)

	author := createTestOwner(t, participants, ctx, "generic-prediction-author")
	other := createTestOwner(t, participants, ctx, "generic-prediction-other")
	community := createTestCommunity(t, communities, ctx, author.ID, "generic-prediction")
	post := createTestPost(t, posts, ctx, community.ID, author.ID, "A falsifiable forecast")

	resolveBy := time.Now().UTC().Add(time.Hour)
	created, err := predictions.UpsertPostPrediction(ctx, &models.Prediction{
		PostID:           post.ID,
		ParticipantID:    author.ID,
		PredictorKind:    "human",
		Subject:          "Toronto records measurable snowfall tomorrow",
		PredictedOutcome: "yes",
		Confidence:       0.8,
		ResolveBy:        resolveBy,
		Reasoning:        "The forecast model shows a cold front.",
	})
	if err != nil {
		t.Fatalf("create post prediction: %v", err)
	}
	if created.ID == "" || created.PredictedOutcome != "yes" || math.Abs(created.Confidence-0.8) > 1e-6 {
		t.Fatalf("created prediction = %#v", created)
	}

	updated, err := predictions.UpsertPostPrediction(ctx, &models.Prediction{
		PostID:           post.ID,
		ParticipantID:    author.ID,
		PredictorKind:    "human",
		Subject:          created.Subject,
		PredictedOutcome: "no",
		Confidence:       0.65,
		ResolveBy:        resolveBy,
		Reasoning:        "The newest model run is warmer.",
	})
	if err != nil {
		t.Fatalf("update post prediction: %v", err)
	}
	if updated.ID != created.ID || updated.PredictedOutcome != "no" || math.Abs(updated.Confidence-0.65) > 1e-6 {
		t.Fatalf("updated prediction = %#v", updated)
	}

	_, err = predictions.UpsertPostPrediction(ctx, &models.Prediction{
		PostID:           post.ID,
		ParticipantID:    other.ID,
		PredictorKind:    "human",
		Subject:          "Someone else's post",
		PredictedOutcome: "yes",
		Confidence:       0.5,
		ResolveBy:        resolveBy,
	})
	if !errors.Is(err, repository.ErrPredictionPostNotOwned) {
		t.Fatalf("other participant upsert error = %v, want ErrPredictionPostNotOwned", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE predictions SET resolve_by = NOW() - INTERVAL '1 minute' WHERE id = $1`, created.ID); err != nil {
		t.Fatalf("move resolve_by into past: %v", err)
	}
	_, err = predictions.UpsertPostPrediction(ctx, &models.Prediction{
		PostID:           post.ID,
		ParticipantID:    author.ID,
		PredictorKind:    "human",
		Subject:          created.Subject,
		PredictedOutcome: "yes",
		Confidence:       0.9,
		ResolveBy:        time.Now().UTC().Add(time.Hour),
	})
	if !errors.Is(err, repository.ErrPredictionLocked) {
		t.Fatalf("post-deadline update error = %v, want ErrPredictionLocked", err)
	}
}

func TestPredictionRepo_ResolutionIsCalibratedIdempotentAndUpdatesStats(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "prediction_stats", "predictions", "posts", "communities", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	predictions := repository.NewPredictionRepo(pool)
	author := createTestOwner(t, participants, ctx, "prediction-resolution")
	community := createTestCommunity(t, communities, ctx, author.ID, "prediction-resolution")
	post := createTestPost(t, posts, ctx, community.ID, author.ID, "Resolve this forecast")

	prediction, err := predictions.UpsertPostPrediction(ctx, &models.Prediction{
		PostID:           post.ID,
		ParticipantID:    author.ID,
		PredictorKind:    "human",
		Subject:          "The launch succeeds",
		PredictedOutcome: "success",
		Confidence:       0.8,
		ResolveBy:        time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create prediction: %v", err)
	}

	if _, _, err := predictions.ResolvePrediction(ctx, prediction.ID, author.ID, "success"); !errors.Is(err, repository.ErrPredictionNotResolvable) {
		t.Fatalf("early resolution error = %v, want ErrPredictionNotResolvable", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE predictions SET resolve_by = NOW() - INTERVAL '1 minute' WHERE id = $1`, prediction.ID); err != nil {
		t.Fatalf("move resolve_by into past: %v", err)
	}

	resolved, changed, err := predictions.ResolvePrediction(ctx, prediction.ID, author.ID, "SUCCESS")
	if err != nil {
		t.Fatalf("resolve prediction: %v", err)
	}
	if !changed || resolved.Outcome == nil || *resolved.Outcome != "correct" || resolved.Brier == nil {
		t.Fatalf("resolved prediction = %#v, changed = %v", resolved, changed)
	}
	if math.Abs(*resolved.Brier-0.04) > 1e-6 {
		t.Fatalf("brier = %v, want 0.04", *resolved.Brier)
	}

	again, changed, err := predictions.ResolvePrediction(ctx, prediction.ID, author.ID, " success ")
	if err != nil || changed || again.ResolvedAt == nil {
		t.Fatalf("idempotent resolution = %#v, changed = %v, err = %v", again, changed, err)
	}
	if _, _, err := predictions.ResolvePrediction(ctx, prediction.ID, author.ID, "failure"); !errors.Is(err, repository.ErrPredictionAlreadyResolved) {
		t.Fatalf("conflicting resolution error = %v, want ErrPredictionAlreadyResolved", err)
	}

	var n, correct, streak int
	var brierSum float64
	if err := pool.QueryRow(ctx, `
		SELECT n, correct, brier_sum::float8, streak
		FROM prediction_stats WHERE participant_id = $1`, author.ID,
	).Scan(&n, &correct, &brierSum, &streak); err != nil {
		t.Fatalf("read prediction stats: %v", err)
	}
	if n != 1 || correct != 1 || streak != 1 || math.Abs(brierSum-0.04) > 1e-6 {
		t.Fatalf("stats n=%d correct=%d brier=%v streak=%d", n, correct, brierSum, streak)
	}

	listed, err := predictions.ListPostPredictions(ctx, post.ID, 20, 0)
	if err != nil || len(listed) != 1 || listed[0].DisplayName != author.DisplayName {
		t.Fatalf("listed predictions = %#v, err = %v", listed, err)
	}
}
