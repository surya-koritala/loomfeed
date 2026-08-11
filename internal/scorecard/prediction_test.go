package scorecard_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
)

func TestCompute_PredictionAccuracyUsesResolvedConfidenceCalibration(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"scorecard_history", "agent_scorecards", "prediction_stats", "predictions",
		"reputation_events", "votes", "posts", "communities", "participants",
	)
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	predictions := repository.NewPredictionRepo(pool)
	author := createScorecardHuman(t, ctx, participants, "prediction-scorecard-author")
	community, err := communities.Create(ctx, &models.Community{
		Name:      "Prediction Scorecard",
		Slug:      "prediction-scorecard",
		CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}

	testCases := []struct {
		title      string
		predicted  string
		resolution string
		confidence float64
	}{
		{title: "Calibrated correct", predicted: "yes", resolution: "yes", confidence: 0.8},
		{title: "Confident wrong", predicted: "yes", resolution: "no", confidence: 0.75},
	}
	for _, tc := range testCases {
		post := createScorecardPost(t, ctx, posts, community.ID, author.ID, tc.title)
		prediction, err := predictions.UpsertPostPrediction(ctx, &models.Prediction{
			PostID:           post.ID,
			ParticipantID:    author.ID,
			PredictorKind:    "human",
			Subject:          tc.title,
			PredictedOutcome: tc.predicted,
			Confidence:       tc.confidence,
			ResolveBy:        time.Now().UTC().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("create %s prediction: %v", tc.title, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE predictions SET resolve_by = NOW() - INTERVAL '1 minute' WHERE id = $1`, prediction.ID); err != nil {
			t.Fatalf("make %s resolvable: %v", tc.title, err)
		}
		if _, _, err := predictions.ResolvePrediction(ctx, prediction.ID, author.ID, tc.resolution); err != nil {
			t.Fatalf("resolve %s: %v", tc.title, err)
		}
	}

	sc, err := scorecard.Compute(ctx, pool, author.ID)
	if err != nil {
		t.Fatalf("compute scorecard: %v", err)
	}
	want := (0.96 + 0.4375) / 2
	if !sc.Signals.PredictionAccuracy.HasData {
		t.Fatal("prediction accuracy should have data after resolution")
	}
	if math.Abs(sc.Signals.PredictionAccuracy.Normalized-want) > 1e-6 {
		t.Fatalf("prediction accuracy = %v, want %v", sc.Signals.PredictionAccuracy.Normalized, want)
	}
}
