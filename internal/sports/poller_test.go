package sports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestPollerTick_UpsertsAndSettles(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "sports_predictions", "sports_prediction_stats", "sports_matches", "participants")

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	// Pre-insert the soon-to-be-FINISHED match (ext_id 524291) so a
	// prediction can reference it. The prediction goes in via direct SQL to
	// sidestep the kickoff lock — tick will upsert this row to FINISHED 2-1.
	matchID, err := repo.UpsertMatch(ctx, &models.SportsMatch{
		ExtID:       524291,
		Competition: "wc2026",
		Stage:       "GROUP_STAGE",
		GroupName:   "Group C",
		HomeTeam:    "Argentina",
		HomeCode:    "ARG",
		AwayTeam:    "France",
		AwayCode:    "FRA",
		KickoffUTC:  time.Now().Add(24 * time.Hour).UTC(),
		Status:      "TIMED",
		Venue:       "SoFi Stadium",
	})
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	human, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Poller Tester"},
		Email:             "poller-tick@example.com",
		PasswordHash:      "hashed_password",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}

	// Payload says Argentina win 2-1, so a "home" pick must grade correct.
	_, err = pool.Exec(ctx, `
		INSERT INTO sports_predictions (match_id, participant_id, predictor_kind, pick)
		VALUES ($1, $2, 'human', 'home')`,
		matchID, human.ID,
	)
	if err != nil {
		t.Fatalf("insert prediction: %v", err)
	}

	srv, _, _, _ := newTestdataServer(t)
	client := NewClient("")
	client.base = srv.URL
	p := NewPoller(client, repo)

	p.tick(ctx)

	// All 3 payload matches must be in the DB by ext_id.
	for _, extID := range []int64{524289, 524290, 524291} {
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM sports_matches WHERE ext_id = $1`, extID,
		).Scan(&count); err != nil {
			t.Fatalf("count match %d: %v", extID, err)
		}
		if count != 1 {
			t.Errorf("expected 1 row for ext_id %d, got %d", extID, count)
		}
	}

	// The FINISHED match must be updated in place and settled.
	got, err := repo.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch: %v", err)
	}
	if got.Status != "FINISHED" {
		t.Errorf("expected status FINISHED, got %q", got.Status)
	}
	if got.HomeScore == nil || *got.HomeScore != 2 || got.AwayScore == nil || *got.AwayScore != 1 {
		t.Errorf("expected score 2-1, got %v-%v", got.HomeScore, got.AwayScore)
	}
	if got.SettledAt == nil {
		t.Error("expected settled_at to be set after tick")
	}

	// The pre-placed prediction must be graded.
	var outcome *string
	if err := pool.QueryRow(ctx,
		`SELECT outcome FROM sports_predictions WHERE match_id = $1 AND participant_id = $2`,
		matchID, human.ID,
	).Scan(&outcome); err != nil {
		t.Fatalf("get prediction outcome: %v", err)
	}
	if outcome == nil {
		t.Fatal("expected prediction outcome to be graded after tick, got NULL")
	}
	if *outcome != "correct" {
		t.Errorf("expected outcome 'correct' for home pick on 2-1, got %q", *outcome)
	}
}

func TestPollerTick_FetchErrorIsFailOpen(t *testing.T) {
	// No DB needed: a failed fetch must return before touching the repo, so
	// a nil-pool repo not panicking proves tick fails open.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("")
	c.base = srv.URL
	p := NewPoller(c, repository.NewSportsRepo(nil))
	p.tick(context.Background()) // must not panic
}
