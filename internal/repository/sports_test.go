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

// sportsTestMatch builds a minimal match for tests; callers override what they need.
func sportsTestMatch(extID int64, kickoff time.Time) *models.SportsMatch {
	return &models.SportsMatch{
		ExtID:       extID,
		Competition: "wc2026",
		Stage:       "GROUP_STAGE",
		GroupName:   "A",
		HomeTeam:    "Mexico",
		HomeCode:    "MEX",
		AwayTeam:    "Canada",
		AwayCode:    "CAN",
		KickoffUTC:  kickoff,
		Status:      "SCHEDULED",
		Venue:       "Estadio Azteca",
	}
}

func floatPtr(f float64) *float64 { return &f }

func TestSportsUpsertMatch_InsertThenUpdate(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	m := sportsTestMatch(1001, time.Now().Add(48*time.Hour).UTC())
	id1, err := repo.UpsertMatch(ctx, m)
	if err != nil {
		t.Fatalf("UpsertMatch insert: %v", err)
	}
	if id1 == "" {
		t.Fatal("expected non-empty match id")
	}

	// Same ext_id again with updated fields must update in place.
	hs, as := 2, 1
	m.Status = "FINISHED"
	m.HomeScore, m.AwayScore = &hs, &as
	m.Venue = "Estadio BBVA"
	id2, err := repo.UpsertMatch(ctx, m)
	if err != nil {
		t.Fatalf("UpsertMatch update: %v", err)
	}
	if id2 != id1 {
		t.Errorf("expected same id on upsert, got %q then %q", id1, id2)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sports_matches`).Scan(&count); err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 match row, got %d", count)
	}

	got, err := repo.GetMatch(ctx, id1)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	if got.Status != "FINISHED" {
		t.Errorf("expected status FINISHED, got %q", got.Status)
	}
	if got.HomeScore == nil || *got.HomeScore != 2 {
		t.Errorf("expected home_score 2, got %v", got.HomeScore)
	}
	if got.AwayScore == nil || *got.AwayScore != 1 {
		t.Errorf("expected away_score 1, got %v", got.AwayScore)
	}
	if got.Venue != "Estadio BBVA" {
		t.Errorf("expected updated venue, got %q", got.Venue)
	}
	if got.ExtID != 1001 {
		t.Errorf("expected ext_id 1001, got %d", got.ExtID)
	}
}

func TestSportsUpsertPrediction_BeforeKickoff(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-bk")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-bk")

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(2001, time.Now().Add(2*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	p := &models.SportsPrediction{
		MatchID:       matchID,
		ParticipantID: agent.ID,
		PredictorKind: "agent",
		HomeProb:      floatPtr(0.5),
		DrawProb:      floatPtr(0.3),
		AwayProb:      floatPtr(0.2),
		Pick:          "home",
		Reasoning:     "home form",
	}
	if err := repo.UpsertPrediction(ctx, p); err != nil {
		t.Fatalf("UpsertPrediction insert: %v", err)
	}

	// Updating the same participant's prediction pre-kickoff must succeed.
	p.HomeProb, p.DrawProb, p.AwayProb = floatPtr(0.2), floatPtr(0.5), floatPtr(0.3)
	p.Pick = "draw"
	p.Reasoning = "revised"
	if err := repo.UpsertPrediction(ctx, p); err != nil {
		t.Fatalf("UpsertPrediction update: %v", err)
	}

	preds, err := repo.ListPredictions(ctx, matchID, 10, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction, got %d", len(preds))
	}
	got := preds[0]
	if got.Pick != "draw" {
		t.Errorf("expected pick 'draw', got %q", got.Pick)
	}
	if got.Reasoning != "revised" {
		t.Errorf("expected reasoning 'revised', got %q", got.Reasoning)
	}
	if got.HomeProb == nil || math.Abs(*got.HomeProb-0.2) > 1e-6 {
		t.Errorf("expected home_prob 0.2, got %v", got.HomeProb)
	}
	if got.DrawProb == nil || math.Abs(*got.DrawProb-0.5) > 1e-6 {
		t.Errorf("expected draw_prob 0.5, got %v", got.DrawProb)
	}
	if got.DisplayName != agent.DisplayName {
		t.Errorf("expected display_name %q, got %q", agent.DisplayName, got.DisplayName)
	}
	if got.StatsN != 0 || got.StatsCorrect != 0 {
		t.Errorf("expected zero stats before settlement, got n=%d correct=%d", got.StatsN, got.StatsCorrect)
	}
}

func TestSportsUpsertPrediction_AfterKickoff(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-ak")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-ak")

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(3001, time.Now().Add(-1*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	p := &models.SportsPrediction{
		MatchID:       matchID,
		ParticipantID: agent.ID,
		PredictorKind: "agent",
		HomeProb:      floatPtr(0.5),
		DrawProb:      floatPtr(0.3),
		AwayProb:      floatPtr(0.2),
		Pick:          "home",
	}
	if err := repo.UpsertPrediction(ctx, p); !errors.Is(err, repository.ErrPredictionLocked) {
		t.Errorf("expected ErrPredictionLocked after kickoff, got %v", err)
	}

	p.MatchID = "00000000-0000-0000-0000-000000000001"
	if err := repo.UpsertPrediction(ctx, p); !errors.Is(err, repository.ErrSportsMatchNotFound) {
		t.Errorf("expected ErrSportsMatchNotFound for unknown match, got %v", err)
	}
}

func TestSportsSettleMatch_GradesAndIsIdempotent(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-settle")
	agent1 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-settle-1")
	agent2 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-settle-2")

	// Kickoff in the future so predictions are accepted, then flip to FINISHED 2-1.
	m := sportsTestMatch(4001, time.Now().Add(2*time.Hour).UTC())
	matchID, err := repo.UpsertMatch(ctx, m)
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	preds := []*models.SportsPrediction{
		{MatchID: matchID, ParticipantID: agent1.ID, PredictorKind: "agent",
			HomeProb: floatPtr(0.7), DrawProb: floatPtr(0.2), AwayProb: floatPtr(0.1), Pick: "home"},
		{MatchID: matchID, ParticipantID: agent2.ID, PredictorKind: "agent",
			HomeProb: floatPtr(0.1), DrawProb: floatPtr(0.2), AwayProb: floatPtr(0.7), Pick: "away"},
		{MatchID: matchID, ParticipantID: owner.ID, PredictorKind: "human", Pick: "home"},
	}
	for i, p := range preds {
		if err := repo.UpsertPrediction(ctx, p); err != nil {
			t.Fatalf("UpsertPrediction %d: %v", i, err)
		}
	}

	hs, as := 2, 1
	m.Status = "FINISHED"
	m.HomeScore, m.AwayScore = &hs, &as
	m.KickoffUTC = time.Now().Add(-3 * time.Hour).UTC()
	if _, err := repo.UpsertMatch(ctx, m); err != nil {
		t.Fatalf("UpsertMatch finish: %v", err)
	}

	if err := repo.SettleMatch(ctx, matchID); err != nil {
		t.Fatalf("SettleMatch: %v", err)
	}

	listed, err := repo.ListPredictions(ctx, matchID, 10, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("expected 3 predictions, got %d", len(listed))
	}
	// Agents first, then humans.
	if listed[0].PredictorKind != "agent" || listed[1].PredictorKind != "agent" || listed[2].PredictorKind != "human" {
		t.Errorf("expected agents before humans, got kinds %q %q %q",
			listed[0].PredictorKind, listed[1].PredictorKind, listed[2].PredictorKind)
	}

	byParticipant := map[string]models.SportsPrediction{}
	for _, p := range listed {
		byParticipant[p.ParticipantID] = p
	}

	p1 := byParticipant[agent1.ID]
	if p1.Outcome == nil || *p1.Outcome != "correct" {
		t.Errorf("agent1: expected outcome correct, got %v", p1.Outcome)
	}
	if p1.Brier == nil || math.Abs(*p1.Brier-0.14) > 1e-6 {
		t.Errorf("agent1: expected brier 0.14, got %v", p1.Brier)
	}

	p2 := byParticipant[agent2.ID]
	if p2.Outcome == nil || *p2.Outcome != "wrong" {
		t.Errorf("agent2: expected outcome wrong, got %v", p2.Outcome)
	}
	if p2.Brier == nil || math.Abs(*p2.Brier-1.34) > 1e-6 {
		t.Errorf("agent2: expected brier 1.34, got %v", p2.Brier)
	}

	ph := byParticipant[owner.ID]
	if ph.Outcome == nil || *ph.Outcome != "correct" {
		t.Errorf("human: expected outcome correct, got %v", ph.Outcome)
	}
	if ph.Brier != nil {
		t.Errorf("human: expected no brier, got %v", ph.Brier)
	}

	type statsRow struct {
		n, correct, streak int
		brierSum           float64
	}
	readStats := func(participantID string) statsRow {
		t.Helper()
		var s statsRow
		err := pool.QueryRow(ctx, `
			SELECT n, correct, brier_sum, streak FROM sports_prediction_stats
			WHERE participant_id = $1`, participantID,
		).Scan(&s.n, &s.correct, &s.brierSum, &s.streak)
		if err != nil {
			t.Fatalf("read stats for %s: %v", participantID, err)
		}
		return s
	}

	s1 := readStats(agent1.ID)
	if s1.n != 1 || s1.correct != 1 || s1.streak != 1 || math.Abs(s1.brierSum-0.14) > 1e-6 {
		t.Errorf("agent1 stats: got %+v", s1)
	}
	s2 := readStats(agent2.ID)
	if s2.n != 1 || s2.correct != 0 || s2.streak != -1 || math.Abs(s2.brierSum-1.34) > 1e-6 {
		t.Errorf("agent2 stats: got %+v", s2)
	}
	sh := readStats(owner.ID)
	if sh.n != 1 || sh.correct != 1 || sh.streak != 1 || sh.brierSum != 0 {
		t.Errorf("human stats: got %+v", sh)
	}

	settled, err := repo.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch after settle: %v", err)
	}
	if settled.SettledAt == nil {
		t.Error("expected settled_at to be set")
	}

	// Settling again must be a no-op.
	if err := repo.SettleMatch(ctx, matchID); err != nil {
		t.Fatalf("SettleMatch second call: %v", err)
	}
	if again := readStats(agent1.ID); again != s1 {
		t.Errorf("agent1 stats changed on second settle: %+v vs %+v", again, s1)
	}
	if again := readStats(agent2.ID); again != s2 {
		t.Errorf("agent2 stats changed on second settle: %+v vs %+v", again, s2)
	}
	if again := readStats(owner.ID); again != sh {
		t.Errorf("human stats changed on second settle: %+v vs %+v", again, sh)
	}
}

func TestSportsLeaderboard_MinN(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-lb")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-lb")

	_, err := pool.Exec(ctx, `
		INSERT INTO sports_prediction_stats (participant_id, predictor_kind, n, correct, brier_sum, streak)
		VALUES ($1, 'agent', 4, 3, 0.8, 2)`, agent.ID)
	if err != nil {
		t.Fatalf("seed stats: %v", err)
	}

	hidden, err := repo.Leaderboard(ctx, "agent", 5, 10)
	if err != nil {
		t.Fatalf("Leaderboard minN=5: %v", err)
	}
	if len(hidden) != 0 {
		t.Errorf("expected 0 rows at minN=5, got %d", len(hidden))
	}

	visible, err := repo.Leaderboard(ctx, "agent", 1, 10)
	if err != nil {
		t.Fatalf("Leaderboard minN=1: %v", err)
	}
	if len(visible) != 1 {
		t.Fatalf("expected 1 row at minN=1, got %d", len(visible))
	}
	row := visible[0]
	if row.ParticipantID != agent.ID {
		t.Errorf("expected participant %q, got %q", agent.ID, row.ParticipantID)
	}
	if row.DisplayName != agent.DisplayName {
		t.Errorf("expected display_name %q, got %q", agent.DisplayName, row.DisplayName)
	}
	if row.N != 4 || row.Correct != 3 {
		t.Errorf("expected n=4 correct=3, got n=%d correct=%d", row.N, row.Correct)
	}
	if math.Abs(row.Accuracy-0.75) > 1e-6 {
		t.Errorf("expected accuracy 0.75, got %v", row.Accuracy)
	}
	if row.AvgBrier == nil || math.Abs(*row.AvgBrier-0.2) > 1e-6 {
		t.Errorf("expected avg_brier 0.2, got %v", row.AvgBrier)
	}
	if row.Streak != 2 {
		t.Errorf("expected streak 2, got %d", row.Streak)
	}
}
