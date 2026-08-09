package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// sportsLiveTables is the cleanup set for live-center tests.
var sportsLiveTables = []string{
	"sports_agent_takes", "sports_match_events",
	"sports_predictions", "sports_prediction_stats",
	"sports_matches", "participants",
}

// seedLiveEvents inserts the canonical three-event timeline used by the
// ordering/limit tests: kickoff (seq 0), a goal (seq 7), half-time (seq 44).
func seedLiveEvents(t *testing.T, repo *repository.SportsRepo, ctx context.Context, matchID string) {
	t.Helper()
	events := []models.SportsMatchEvent{
		{Seq: 0, Kind: "play", Body: "Kickoff."},
		{Seq: 7, Kind: "goal", Minute: strPtr("9'"), Side: strPtr("home"),
			Player: strPtr("Hirving Lozano"), Body: "Goal! Mexico 1, South Africa 0."},
		{Seq: 44, Kind: "ht", Body: "First Half ends."},
	}
	if err := repo.UpsertEvents(ctx, matchID, events); err != nil {
		t.Fatalf("UpsertEvents seed: %v", err)
	}
}

// describeTimeline flattens a timeline into "kind:seq" strings for exact
// order assertions ("take:nil" for pre-match takes).
func describeTimeline(items []models.SportsTimelineItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch it.Kind {
		case "event":
			out = append(out, fmt.Sprintf("event:%d", it.Event.Seq))
		case "take":
			if it.Take.EventSeq == nil {
				out = append(out, "take:nil")
			} else {
				out = append(out, fmt.Sprintf("take:%d", *it.Take.EventSeq))
			}
		default:
			out = append(out, "unknown:"+it.Kind)
		}
	}
	return out
}

func assertTimelineOrder(t *testing.T, got []models.SportsTimelineItem, want []string) {
	t.Helper()
	desc := describeTimeline(got)
	if len(desc) != len(want) {
		t.Fatalf("expected %d timeline items %v, got %d: %v", len(want), want, len(desc), desc)
	}
	for i := range want {
		if desc[i] != want[i] {
			t.Fatalf("timeline order mismatch at %d: expected %v, got %v", i, want, desc)
		}
	}
}

func TestSportsRepo_UpsertEventsIdempotent(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5001, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	if seq, err := repo.MaxEventSeq(ctx, matchID); err != nil || seq != -1 {
		t.Fatalf("MaxEventSeq before events: expected -1/nil, got %d/%v", seq, err)
	}

	events := []models.SportsMatchEvent{
		{Seq: 0, Kind: "play", Body: "Kickoff."},
		{Seq: 7, Kind: "play", Minute: strPtr("9'"), Side: strPtr("home"),
			Body: "Goal! Mexico 1, South Africa 0."},
		{Seq: 44, Kind: "ht", Body: "First Half ends."},
	}
	if err := repo.UpsertEvents(ctx, matchID, events); err != nil {
		t.Fatalf("UpsertEvents first: %v", err)
	}

	// Re-upserting the same batch with one body changed must update in place,
	// not duplicate. ESPN re-polls can also reclassify an entry (a generic
	// "play" later upgraded to "goal" with the scorer attached); the conflict
	// update must refresh kind/side/player too, not just body.
	events[0].Body = "Kickoff!"
	events[1].Kind = "goal"
	events[1].Player = strPtr("Hirving Lozano")
	if err := repo.UpsertEvents(ctx, matchID, events); err != nil {
		t.Fatalf("UpsertEvents second: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sports_match_events`).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 event rows after re-upsert, got %d", count)
	}

	var body string
	err = pool.QueryRow(ctx, `
		SELECT body FROM sports_match_events WHERE match_id = $1 AND seq = 0`, matchID,
	).Scan(&body)
	if err != nil {
		t.Fatalf("read seq 0 body: %v", err)
	}
	if body != "Kickoff!" {
		t.Errorf("expected seq 0 body updated to %q, got %q", "Kickoff!", body)
	}

	var kind string
	var player *string
	err = pool.QueryRow(ctx, `
		SELECT kind, player FROM sports_match_events WHERE match_id = $1 AND seq = 7`, matchID,
	).Scan(&kind, &player)
	if err != nil {
		t.Fatalf("read seq 7 kind/player: %v", err)
	}
	if kind != "goal" {
		t.Errorf("expected seq 7 kind reclassified to %q, got %q", "goal", kind)
	}
	if player == nil || *player != "Hirving Lozano" {
		t.Errorf("expected seq 7 player updated to %q, got %v", "Hirving Lozano", player)
	}

	if seq, err := repo.MaxEventSeq(ctx, matchID); err != nil || seq != 44 {
		t.Errorf("MaxEventSeq after events: expected 44/nil, got %d/%v", seq, err)
	}
}

func TestSportsRepo_TimelineOrdering(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-tl")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-tl")

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5101, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	seedLiveEvents(t, repo, ctx, matchID)

	// Takes are inserted AFTER the events; ordering must come from the merge
	// key, not insertion time. The pre-match take (event_seq NULL → -1) sorts
	// before event seq 0; the seq-7 take lands right after event 7.
	goalTake := &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID, EventSeq: intPtr(7), Body: "What a strike.",
	}
	if err := repo.InsertTake(ctx, goalTake); err != nil {
		t.Fatalf("InsertTake (seq 7): %v", err)
	}
	preTake := &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID, Body: "Pre-match read: home edge.",
	}
	if err := repo.InsertTake(ctx, preTake); err != nil {
		t.Fatalf("InsertTake (nil): %v", err)
	}
	if goalTake.ID == "" || goalTake.CreatedAt.IsZero() {
		t.Errorf("InsertTake must populate ID and CreatedAt, got id=%q created_at=%v",
			goalTake.ID, goalTake.CreatedAt)
	}

	items, err := repo.Timeline(ctx, matchID, 50)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	assertTimelineOrder(t, items, []string{"take:nil", "event:0", "event:7", "take:7", "event:44"})

	if items[1].Event.Body != "Kickoff." {
		t.Errorf("expected event 0 body %q, got %q", "Kickoff.", items[1].Event.Body)
	}
	if items[3].Take.Body != "What a strike." {
		t.Errorf("expected seq-7 take body %q, got %q", "What a strike.", items[3].Take.Body)
	}
}

func TestSportsRepo_TimelineLimit(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-tll")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-tll")

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5201, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	seedLiveEvents(t, repo, ctx, matchID)
	for _, take := range []*models.SportsAgentTake{
		{MatchID: matchID, ParticipantID: agent.ID, Body: "Pre-match."},
		{MatchID: matchID, ParticipantID: agent.ID, EventSeq: intPtr(7), Body: "Goal reaction."},
	} {
		if err := repo.InsertTake(ctx, take); err != nil {
			t.Fatalf("InsertTake: %v", err)
		}
	}

	// 5 items total; limit 2 keeps the most recent window, still ascending.
	items, err := repo.Timeline(ctx, matchID, 2)
	if err != nil {
		t.Fatalf("Timeline limit 2: %v", err)
	}
	assertTimelineOrder(t, items, []string{"take:7", "event:44"})
}

func TestSportsRepo_TakeJoinsPrediction(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-tj")
	agent1 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-tj-1")
	agent2 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-tj-2")

	// Kickoff in the future so the prediction is accepted.
	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5301, time.Now().Add(2*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	if err := repo.UpsertPrediction(ctx, &models.SportsPrediction{
		MatchID: matchID, ParticipantID: agent1.ID, PredictorKind: "agent",
		HomeProb: floatPtr(0.6), DrawProb: floatPtr(0.25), AwayProb: floatPtr(0.15),
		Pick: "home",
	}); err != nil {
		t.Fatalf("UpsertPrediction: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE sports_predictions SET outcome = 'correct'
		WHERE match_id = $1 AND participant_id = $2`, matchID, agent1.ID); err != nil {
		t.Fatalf("set outcome: %v", err)
	}

	take1 := &models.SportsAgentTake{MatchID: matchID, ParticipantID: agent1.ID, Body: "Backing the hosts."}
	if err := repo.InsertTake(ctx, take1); err != nil {
		t.Fatalf("InsertTake agent1: %v", err)
	}
	take2 := &models.SportsAgentTake{MatchID: matchID, ParticipantID: agent2.ID, Body: "No call from me."}
	if err := repo.InsertTake(ctx, take2); err != nil {
		t.Fatalf("InsertTake agent2: %v", err)
	}

	checkTakes := func(label string, takes map[string]models.SportsAgentTake) {
		t.Helper()
		g1, ok := takes[agent1.ID]
		if !ok {
			t.Fatalf("%s: missing take for agent1", label)
		}
		if g1.DisplayName != agent1.DisplayName {
			t.Errorf("%s agent1: expected display_name %q, got %q", label, agent1.DisplayName, g1.DisplayName)
		}
		if g1.Pick == nil || *g1.Pick != "home" {
			t.Errorf("%s agent1: expected pick 'home', got %v", label, g1.Pick)
		}
		if g1.Outcome == nil || *g1.Outcome != "correct" {
			t.Errorf("%s agent1: expected outcome 'correct', got %v", label, g1.Outcome)
		}

		g2, ok := takes[agent2.ID]
		if !ok {
			t.Fatalf("%s: missing take for agent2", label)
		}
		if g2.DisplayName != agent2.DisplayName {
			t.Errorf("%s agent2: expected display_name %q, got %q", label, agent2.DisplayName, g2.DisplayName)
		}
		if g2.Pick != nil {
			t.Errorf("%s agent2: expected nil pick without prediction, got %v", label, *g2.Pick)
		}
		if g2.Outcome != nil {
			t.Errorf("%s agent2: expected nil outcome without prediction, got %v", label, *g2.Outcome)
		}
	}

	items, err := repo.Timeline(ctx, matchID, 50)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 timeline items, got %d", len(items))
	}
	fromTimeline := map[string]models.SportsAgentTake{}
	for _, it := range items {
		if it.Kind != "take" || it.Take == nil {
			t.Fatalf("expected only take items, got kind %q", it.Kind)
		}
		fromTimeline[it.Take.ParticipantID] = *it.Take
	}
	checkTakes("Timeline", fromTimeline)

	latest, err := repo.LatestTakes(ctx, 10)
	if err != nil {
		t.Fatalf("LatestTakes: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest takes, got %d", len(latest))
	}
	if latest[0].ParticipantID != agent2.ID {
		t.Errorf("expected newest take (agent2) first, got participant %q", latest[0].ParticipantID)
	}
	fromLatest := map[string]models.SportsAgentTake{}
	for _, tk := range latest {
		fromLatest[tk.ParticipantID] = tk
	}
	checkTakes("LatestTakes", fromLatest)
}

func TestSportsRepo_EventsSince(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5401, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	otherID, err := repo.UpsertMatch(ctx, sportsTestMatch(5402, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch other: %v", err)
	}

	// Mix of 'play' filler and key events; EventsSince must drop the
	// filler, respect afterSeq and return ascending.
	events := []models.SportsMatchEvent{
		{Seq: 0, Kind: "play", Body: "Kickoff."},
		{Seq: 7, Kind: "goal", Minute: strPtr("9'"), Body: "Goal! Mexico 1, Canada 0."},
		{Seq: 20, Kind: "play", Body: "Corner, Canada."},
		{Seq: 44, Kind: "ht", Body: "First Half ends."},
		{Seq: 50, Kind: "card", Body: "Yellow card for Johnson."},
	}
	if err := repo.UpsertEvents(ctx, matchID, events); err != nil {
		t.Fatalf("UpsertEvents: %v", err)
	}
	// Another match's key event must never leak in.
	if err := repo.UpsertEvents(ctx, otherID, []models.SportsMatchEvent{
		{Seq: 3, Kind: "goal", Body: "Goal in the other match."},
	}); err != nil {
		t.Fatalf("UpsertEvents other: %v", err)
	}

	assertSeqs := func(label string, got []models.SportsMatchEvent, want []int) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: expected seqs %v, got %d events: %+v", label, want, len(got), got)
		}
		for i := range want {
			if got[i].Seq != want[i] {
				t.Fatalf("%s: expected seqs %v, got seq %d at index %d", label, want, got[i].Seq, i)
			}
			if got[i].Kind == "play" {
				t.Errorf("%s: 'play' event leaked through at seq %d", label, got[i].Seq)
			}
		}
	}

	all, err := repo.EventsSince(ctx, matchID, -1)
	if err != nil {
		t.Fatalf("EventsSince(-1): %v", err)
	}
	assertSeqs("afterSeq -1", all, []int{7, 44, 50})
	if all[0].Kind != "goal" || all[0].Body != "Goal! Mexico 1, Canada 0." {
		t.Errorf("expected goal event first, got kind %q body %q", all[0].Kind, all[0].Body)
	}
	if all[0].Minute == nil || *all[0].Minute != "9'" {
		t.Errorf("expected minute 9' on the goal, got %v", all[0].Minute)
	}

	later, err := repo.EventsSince(ctx, matchID, 7)
	if err != nil {
		t.Fatalf("EventsSince(7): %v", err)
	}
	assertSeqs("afterSeq 7", later, []int{44, 50})

	none, err := repo.EventsSince(ctx, matchID, 50)
	if err != nil {
		t.Fatalf("EventsSince(50): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected no events after seq 50, got %d", len(none))
	}
}

func TestSportsRepo_MaxTakeSeq(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-mts")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "sports-mts")

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(5501, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	if seq, err := repo.MaxTakeSeq(ctx, matchID); err != nil || seq != -1 {
		t.Fatalf("MaxTakeSeq with no takes: expected -1/nil, got %d/%v", seq, err)
	}

	// A pre-match take (NULL event_seq) must be ignored, not break the MAX.
	if err := repo.InsertTake(ctx, &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID, Body: "Pre-match read.",
	}); err != nil {
		t.Fatalf("InsertTake (nil seq): %v", err)
	}
	if seq, err := repo.MaxTakeSeq(ctx, matchID); err != nil || seq != -1 {
		t.Fatalf("MaxTakeSeq with only NULL-seq take: expected -1/nil, got %d/%v", seq, err)
	}

	if err := repo.InsertTake(ctx, &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID, EventSeq: intPtr(7), Body: "Goal reaction.",
	}); err != nil {
		t.Fatalf("InsertTake (seq 7): %v", err)
	}
	if seq, err := repo.MaxTakeSeq(ctx, matchID); err != nil || seq != 7 {
		t.Fatalf("MaxTakeSeq after seq-7 take: expected 7/nil, got %d/%v", seq, err)
	}

	if err := repo.InsertTake(ctx, &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID, EventSeq: intPtr(44), Body: "HT verdict.",
	}); err != nil {
		t.Fatalf("InsertTake (seq 44): %v", err)
	}
	if seq, err := repo.MaxTakeSeq(ctx, matchID); err != nil || seq != 44 {
		t.Fatalf("MaxTakeSeq after seq-44 take: expected 44/nil, got %d/%v", seq, err)
	}
}

func TestSportsRepo_GroupStandings(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	finished := func(extID int64, group, stage, home, hcode, away, acode string, hs, as int) *models.SportsMatch {
		return &models.SportsMatch{
			ExtID: extID, Competition: "wc2026", Stage: stage, GroupName: group,
			HomeTeam: home, HomeCode: hcode, AwayTeam: away, AwayCode: acode,
			KickoffUTC: time.Now().Add(-24 * time.Hour).UTC(), Status: "FINISHED",
			HomeScore: &hs, AwayScore: &as,
		}
	}

	seeds := []*models.SportsMatch{
		finished(6001, "Group A", "GROUP_STAGE", "Mexico", "MEX", "South Africa", "RSA", 2, 0),
		finished(6002, "Group B", "GROUP_STAGE", "South Korea", "KOR", "Czechia", "CZE", 2, 1),
		// Not finished: must not count.
		{ExtID: 6003, Competition: "wc2026", Stage: "GROUP_STAGE", GroupName: "Group A",
			HomeTeam: "Mexico", HomeCode: "MEX", AwayTeam: "Canada", AwayCode: "CAN",
			KickoffUTC: time.Now().Add(24 * time.Hour).UTC(), Status: "TIMED"},
		// Finished but knockout: must not count.
		finished(6004, "", "ROUND_OF_16", "Brazil", "BRA", "Argentina", "ARG", 3, 1),
	}
	for _, m := range seeds {
		if _, err := repo.UpsertMatch(ctx, m); err != nil {
			t.Fatalf("UpsertMatch %d: %v", m.ExtID, err)
		}
	}

	rows, err := repo.GroupStandings(ctx)
	if err != nil {
		t.Fatalf("GroupStandings: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 standing rows (2 per group), got %d: %+v", len(rows), rows)
	}

	want := []models.SportsStandingRow{
		{GroupName: "Group A", Team: "Mexico", Code: "MEX",
			Played: 1, Won: 1, Drawn: 0, Lost: 0, GF: 2, GA: 0, GD: 2, Points: 3},
		{GroupName: "Group A", Team: "South Africa", Code: "RSA",
			Played: 1, Won: 0, Drawn: 0, Lost: 1, GF: 0, GA: 2, GD: -2, Points: 0},
		{GroupName: "Group B", Team: "South Korea", Code: "KOR",
			Played: 1, Won: 1, Drawn: 0, Lost: 0, GF: 2, GA: 1, GD: 1, Points: 3},
		{GroupName: "Group B", Team: "Czechia", Code: "CZE",
			Played: 1, Won: 0, Drawn: 0, Lost: 1, GF: 1, GA: 2, GD: -1, Points: 0},
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("standings row %d: expected %+v, got %+v", i, want[i], rows[i])
		}
	}
}

func TestSportsRepo_ListMatchesPredictionCount(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sports-pc")
	agent1 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-pc-1")
	agent2 := createTestAgent(t, pRepo, ctx, owner.ID, "sports-pc-2")

	// Kickoffs in the future so predictions are accepted.
	matchA, err := repo.UpsertMatch(ctx, sportsTestMatch(9001, time.Now().Add(2*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch A: %v", err)
	}
	matchB, err := repo.UpsertMatch(ctx, sportsTestMatch(9002, time.Now().Add(3*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch B: %v", err)
	}

	// Match A: two agent predictions plus one human; only agents count.
	preds := []*models.SportsPrediction{
		{MatchID: matchA, ParticipantID: agent1.ID, PredictorKind: "agent",
			HomeProb: floatPtr(0.6), DrawProb: floatPtr(0.25), AwayProb: floatPtr(0.15), Pick: "home"},
		{MatchID: matchA, ParticipantID: agent2.ID, PredictorKind: "agent",
			HomeProb: floatPtr(0.2), DrawProb: floatPtr(0.3), AwayProb: floatPtr(0.5), Pick: "away"},
		{MatchID: matchA, ParticipantID: owner.ID, PredictorKind: "human", Pick: "home"},
	}
	for i, p := range preds {
		if err := repo.UpsertPrediction(ctx, p); err != nil {
			t.Fatalf("UpsertPrediction %d: %v", i, err)
		}
	}

	listed, err := repo.ListMatches(ctx, "wc2026", "", "", "")
	if err != nil {
		t.Fatalf("ListMatches: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 listed matches, got %d", len(listed))
	}
	byID := map[string]models.SportsMatch{}
	for _, m := range listed {
		byID[m.ID] = m
	}
	if got := byID[matchA].PredictionCount; got != 2 {
		t.Errorf("match A: expected prediction_count 2 (agents only), got %d", got)
	}
	if got := byID[matchB].PredictionCount; got != 0 {
		t.Errorf("match B: expected prediction_count 0, got %d", got)
	}
}

func TestSportsRepo_MatchesToEnrich(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	now := time.Now().UTC()
	seed := func(extID int64, status string, kickoff time.Time) {
		t.Helper()
		m := sportsTestMatch(extID, kickoff)
		m.Status = status
		if status == "FINISHED" {
			hs, as := 1, 0
			m.HomeScore, m.AwayScore = &hs, &as
		}
		if _, err := repo.UpsertMatch(ctx, m); err != nil {
			t.Fatalf("UpsertMatch %d: %v", extID, err)
		}
	}

	seed(7001, "IN_PLAY", now.Add(-30*time.Minute))           // live → included
	seed(7002, "TIMED", now.Add(time.Hour))                   // kicks off in 1h → included
	seed(7003, "FINISHED", now.Add(-150*time.Minute))         // ended 2.5h after kickoff → included
	seed(7004, "FINISHED", now.Add(-26*time.Hour))            // long over → excluded
	seed(7005, "TIMED", now.Add(26*time.Hour))                // far future → excluded

	matches, err := repo.MatchesToEnrich(ctx)
	if err != nil {
		t.Fatalf("MatchesToEnrich: %v", err)
	}

	gotExt := make([]int64, len(matches))
	for i, m := range matches {
		gotExt[i] = m.ExtID
	}
	wantExt := []int64{7003, 7001, 7002} // kickoff_utc ASC
	if len(gotExt) != len(wantExt) {
		t.Fatalf("expected matches %v, got %v", wantExt, gotExt)
	}
	for i := range wantExt {
		if gotExt[i] != wantExt[i] {
			t.Fatalf("expected matches %v in kickoff order, got %v", wantExt, gotExt)
		}
	}
}

func TestSportsRepo_SetEnrichment(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, sportsLiveTables...)

	repo := repository.NewSportsRepo(pool)
	ctx := context.Background()

	matchID, err := repo.UpsertMatch(ctx, sportsTestMatch(8001, time.Now().Add(-time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	got, err := repo.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch before enrichment: %v", err)
	}
	if got.ESPNEventID != nil || got.Lineups != nil {
		t.Errorf("expected nil enrichment fields before SetEnrichment, got %v / %s",
			got.ESPNEventID, got.Lineups)
	}

	lineups := []byte(`{"home":[{"name":"Guillermo Ochoa"}],"away":[]}`)
	if err := repo.SetEnrichment(ctx, matchID, 401234, lineups); err != nil {
		t.Fatalf("SetEnrichment: %v", err)
	}

	assertLineups := func(label string, raw json.RawMessage) {
		t.Helper()
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("%s: lineups not valid JSON (%v): %s", label, err, raw)
		}
		if _, ok := parsed["home"]; !ok {
			t.Errorf("%s: expected lineups to keep 'home' key, got %s", label, raw)
		}
	}

	got, err = repo.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch after enrichment: %v", err)
	}
	if got.ESPNEventID == nil || *got.ESPNEventID != 401234 {
		t.Errorf("expected espn_event_id 401234, got %v", got.ESPNEventID)
	}
	assertLineups("first set", got.Lineups)

	// nil lineups must keep the stored JSON (COALESCE) while updating the id.
	if err := repo.SetEnrichment(ctx, matchID, 401235, nil); err != nil {
		t.Fatalf("SetEnrichment nil lineups: %v", err)
	}
	got, err = repo.GetMatch(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatch after nil-lineups enrichment: %v", err)
	}
	if got.ESPNEventID == nil || *got.ESPNEventID != 401235 {
		t.Errorf("expected espn_event_id updated to 401235, got %v", got.ESPNEventID)
	}
	assertLineups("after nil-lineups set", got.Lineups)

	// ListMatches must scan the new columns too.
	listed, err := repo.ListMatches(ctx, "wc2026", "", "", "")
	if err != nil {
		t.Fatalf("ListMatches with enrichment columns: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed match, got %d", len(listed))
	}
	if listed[0].ESPNEventID == nil || *listed[0].ESPNEventID != 401235 {
		t.Errorf("ListMatches: expected espn_event_id 401235, got %v", listed[0].ESPNEventID)
	}
	assertLineups("ListMatches", listed[0].Lineups)
}
