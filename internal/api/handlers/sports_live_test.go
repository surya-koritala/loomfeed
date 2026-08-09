package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

// setupSportsLiveTest is setupSportsTest plus the live-center tables
// (events + takes) in the cleanup set.
func setupSportsLiveTest(t *testing.T) (*handlers.SportsHandler, *repository.SportsRepo, *repository.ParticipantRepo) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"sports_agent_takes", "sports_match_events",
		"sports_predictions", "sports_prediction_stats", "sports_matches",
		"agent_identities", "human_users", "participants")
	sportsRepo := repository.NewSportsRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	return handlers.NewSportsHandler(sportsRepo), sportsRepo, participants
}

func sportsStrPtr(s string) *string { return &s }
func sportsIntPtr(i int) *int       { return &i }
func sportsFloatPtr(f float64) *float64 {
	return &f
}

// getTimeline hits the Timeline handler for a match id with an optional
// raw query string ("" or e.g. "limit=2").
func getTimeline(t *testing.T, h *handlers.SportsHandler, matchID, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/sports/matches/" + matchID + "/timeline"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("id", matchID)
	rec := httptest.NewRecorder()
	h.Timeline(rec, req)
	return rec
}

func decodeTimeline(t *testing.T, rec *httptest.ResponseRecorder) []models.SportsTimelineItem {
	t.Helper()
	var resp struct {
		Data []models.SportsTimelineItem `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	return resp.Data
}

// timelineKinds flattens items into "kind:seq" strings for order assertions.
func timelineKinds(t *testing.T, items []models.SportsTimelineItem) []string {
	t.Helper()
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch it.Kind {
		case "event":
			out = append(out, "event:"+itoa(it.Event.Seq))
		case "take":
			if it.Take.EventSeq == nil {
				out = append(out, "take:nil")
			} else {
				out = append(out, "take:"+itoa(*it.Take.EventSeq))
			}
		default:
			t.Fatalf("unexpected timeline kind %q", it.Kind)
		}
	}
	return out
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// --- GET timeline ---

func TestSportsTimeline_MergedOrderTakeFieldsAndLimits(t *testing.T) {
	h, sportsRepo, participants := setupSportsLiveTest(t)
	ctx := context.Background()
	agent := createSportsAgent(t, participants, "timeline")
	// Kickoff in the future so the prediction upsert is accepted.
	matchID := createSportsMatch(t, sportsRepo, 9001, time.Now().Add(2*time.Hour).UTC())

	if err := sportsRepo.UpsertEvents(ctx, matchID, []models.SportsMatchEvent{
		{Seq: 0, Kind: "play", Body: "Kickoff."},
		{Seq: 7, Kind: "goal", Minute: sportsStrPtr("9'"), Side: sportsStrPtr("home"),
			Player: sportsStrPtr("Hirving Lozano"), Body: "Goal! Mexico 1, Canada 0."},
	}); err != nil {
		t.Fatalf("UpsertEvents: %v", err)
	}
	if err := sportsRepo.UpsertPrediction(ctx, &models.SportsPrediction{
		MatchID: matchID, ParticipantID: agent.ID, PredictorKind: "agent",
		HomeProb: sportsFloatPtr(0.6), DrawProb: sportsFloatPtr(0.25), AwayProb: sportsFloatPtr(0.15),
		Pick: "home",
	}); err != nil {
		t.Fatalf("UpsertPrediction: %v", err)
	}
	if err := sportsRepo.InsertTake(ctx, &models.SportsAgentTake{
		MatchID: matchID, ParticipantID: agent.ID,
		EventSeq: sportsIntPtr(7), Body: "Lozano again — called it.",
	}); err != nil {
		t.Fatalf("InsertTake: %v", err)
	}

	rec := getTimeline(t, h, matchID, "")
	testutil.AssertStatus(t, rec, http.StatusOK)
	items := decodeTimeline(t, rec)
	want := []string{"event:0", "event:7", "take:7"}
	if got := timelineKinds(t, items); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected ASC merged order %v, got %v", want, got)
	}

	// The take item carries the joined display fields.
	take := items[2].Take
	if take.DisplayName != agent.DisplayName {
		t.Errorf("expected take display_name %q, got %q", agent.DisplayName, take.DisplayName)
	}
	if take.Pick == nil || *take.Pick != "home" {
		t.Errorf("expected take pick 'home', got %v", take.Pick)
	}
	if take.Body != "Lozano again — called it." {
		t.Errorf("expected take body preserved, got %q", take.Body)
	}

	// limit=2 keeps the most recent window, still ascending.
	rec = getTimeline(t, h, matchID, "limit=2")
	testutil.AssertStatus(t, rec, http.StatusOK)
	want = []string{"event:7", "take:7"}
	if got := timelineKinds(t, decodeTimeline(t, rec)); !reflect.DeepEqual(got, want) {
		t.Errorf("limit=2: expected %v, got %v", want, got)
	}

	// limit=0 clamps up to 1.
	rec = getTimeline(t, h, matchID, "limit=0")
	testutil.AssertStatus(t, rec, http.StatusOK)
	if got := decodeTimeline(t, rec); len(got) != 1 {
		t.Errorf("limit=0: expected clamp to 1 item, got %d", len(got))
	}

	// limit=9999 clamps down to 300 — all 3 items here.
	rec = getTimeline(t, h, matchID, "limit=9999")
	testutil.AssertStatus(t, rec, http.StatusOK)
	if got := decodeTimeline(t, rec); len(got) != 3 {
		t.Errorf("limit=9999: expected all 3 items, got %d", len(got))
	}
}

func TestSportsTimeline_UnknownAndMalformedID(t *testing.T) {
	h, _, _ := setupSportsLiveTest(t)

	// Unknown-but-valid uuid: 200 + empty array (mirrors ListPredictions).
	rec := getTimeline(t, h, "00000000-0000-0000-0000-000000000001", "")
	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp struct {
		Data []models.SportsTimelineItem `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data == nil {
		t.Error("expected non-null empty data array for unknown match")
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 items for unknown match, got %d", len(resp.Data))
	}

	// Malformed uuid: 404 (mirrors GetMatch/ListPredictions).
	rec = getTimeline(t, h, "not-a-uuid", "")
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

// --- GET lineups ---

func getLineups(t *testing.T, h *handlers.SportsHandler, matchID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/"+matchID+"/lineups", nil)
	req.SetPathValue("id", matchID)
	rec := httptest.NewRecorder()
	h.Lineups(rec, req)
	return rec
}

func TestSportsLineups_PresentAbsentAndNotFound(t *testing.T) {
	h, sportsRepo, _ := setupSportsLiveTest(t)
	ctx := context.Background()

	withID := createSportsMatch(t, sportsRepo, 9101, time.Now().Add(-time.Hour).UTC())
	lineups := `{"home":[{"name":"Guillermo Ochoa","pos":"G"}],"away":[{"name":"Alphonso Davies","pos":"D"}]}`
	if err := sportsRepo.SetEnrichment(ctx, withID, 401234, []byte(lineups)); err != nil {
		t.Fatalf("SetEnrichment: %v", err)
	}

	rec := getLineups(t, h, withID)
	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	// jsonb round-trips may reorder keys/whitespace; compare semantically.
	var got, want any
	if err := json.Unmarshal(resp.Data, &got); err != nil {
		t.Fatalf("lineups data not valid JSON (%v): %s", err, resp.Data)
	}
	if err := json.Unmarshal([]byte(lineups), &want); err != nil {
		t.Fatalf("unmarshal seed lineups: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected lineups %s, got %s", lineups, resp.Data)
	}

	// Match without lineups: 200 with null data.
	withoutID := createSportsMatch(t, sportsRepo, 9102, time.Now().Add(-time.Hour).UTC())
	rec = getLineups(t, h, withoutID)
	testutil.AssertStatus(t, rec, http.StatusOK)
	var nullResp struct {
		Data json.RawMessage `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &nullResp)
	if string(nullResp.Data) != "null" {
		t.Errorf("expected null lineups data, got %s", nullResp.Data)
	}

	// Unknown id: 404, same as GetMatch.
	rec = getLineups(t, h, "00000000-0000-0000-0000-000000000001")
	testutil.AssertStatus(t, rec, http.StatusNotFound)

	// Malformed id: 404, same as GetMatch.
	rec = getLineups(t, h, "not-a-uuid")
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

// --- GET standings ---

func TestSportsStandings_ComputedFromFinishedGroupMatches(t *testing.T) {
	h, sportsRepo, _ := setupSportsLiveTest(t)
	ctx := context.Background()

	finished := func(extID int64, group, home, hcode, away, acode string, hs, as int) *models.SportsMatch {
		return &models.SportsMatch{
			ExtID: extID, Competition: "wc2026", Stage: "GROUP_STAGE", GroupName: group,
			HomeTeam: home, HomeCode: hcode, AwayTeam: away, AwayCode: acode,
			KickoffUTC: time.Now().Add(-24 * time.Hour).UTC(), Status: "FINISHED",
			HomeScore: &hs, AwayScore: &as,
		}
	}
	seeds := []*models.SportsMatch{
		finished(9201, "Group A", "Mexico", "MEX", "South Africa", "RSA", 2, 0),
		finished(9202, "Group A", "Mexico", "MEX", "Canada", "CAN", 1, 1),
	}
	for _, m := range seeds {
		if _, err := sportsRepo.UpsertMatch(ctx, m); err != nil {
			t.Fatalf("UpsertMatch %d: %v", m.ExtID, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/standings", nil)
	rec := httptest.NewRecorder()
	h.Standings(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data []models.SportsStandingRow `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	want := []models.SportsStandingRow{
		{GroupName: "Group A", Team: "Mexico", Code: "MEX",
			Played: 2, Won: 1, Drawn: 1, Lost: 0, GF: 3, GA: 1, GD: 2, Points: 4},
		{GroupName: "Group A", Team: "Canada", Code: "CAN",
			Played: 1, Won: 0, Drawn: 1, Lost: 0, GF: 1, GA: 1, GD: 0, Points: 1},
		{GroupName: "Group A", Team: "South Africa", Code: "RSA",
			Played: 1, Won: 0, Drawn: 0, Lost: 1, GF: 0, GA: 2, GD: -2, Points: 0},
	}
	if !reflect.DeepEqual(resp.Data, want) {
		t.Errorf("expected standings %+v, got %+v", want, resp.Data)
	}
}

// --- GET live takes ---

func getLiveTakes(t *testing.T, h *handlers.SportsHandler, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/sports/takes/live"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	h.LiveTakes(rec, req)
	return rec
}

func decodeLiveTakes(t *testing.T, rec *httptest.ResponseRecorder) []models.SportsAgentTake {
	t.Helper()
	var resp struct {
		Data []models.SportsAgentTake `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	return resp.Data
}

func TestSportsLiveTakes_NewestFirstAndLimits(t *testing.T) {
	h, sportsRepo, participants := setupSportsLiveTest(t)
	ctx := context.Background()
	agent := createSportsAgent(t, participants, "livetakes")
	matchA := createSportsMatch(t, sportsRepo, 9301, time.Now().Add(2*time.Hour).UTC())
	matchB := createSportsMatch(t, sportsRepo, 9302, time.Now().Add(3*time.Hour).UTC())

	bodies := []string{"first take", "second take", "third take"}
	for i, m := range []string{matchA, matchB, matchA} {
		if err := sportsRepo.InsertTake(ctx, &models.SportsAgentTake{
			MatchID: m, ParticipantID: agent.ID, Body: bodies[i],
		}); err != nil {
			t.Fatalf("InsertTake %d: %v", i, err)
		}
	}

	rec := getLiveTakes(t, h, "")
	testutil.AssertStatus(t, rec, http.StatusOK)
	takes := decodeLiveTakes(t, rec)
	if len(takes) != 3 {
		t.Fatalf("expected 3 takes, got %d", len(takes))
	}
	for i, wantBody := range []string{"third take", "second take", "first take"} {
		if takes[i].Body != wantBody {
			t.Errorf("take %d: expected %q (newest first), got %q", i, wantBody, takes[i].Body)
		}
	}
	if takes[0].DisplayName != agent.DisplayName {
		t.Errorf("expected display_name %q joined, got %q", agent.DisplayName, takes[0].DisplayName)
	}

	rec = getLiveTakes(t, h, "limit=2")
	testutil.AssertStatus(t, rec, http.StatusOK)
	takes = decodeLiveTakes(t, rec)
	if len(takes) != 2 || takes[0].Body != "third take" || takes[1].Body != "second take" {
		t.Errorf("limit=2: expected the 2 newest takes, got %+v", takes)
	}

	// limit=0 clamps up to 1; limit=9999 clamps down to 20 (all 3 here).
	rec = getLiveTakes(t, h, "limit=0")
	testutil.AssertStatus(t, rec, http.StatusOK)
	if takes = decodeLiveTakes(t, rec); len(takes) != 1 {
		t.Errorf("limit=0: expected clamp to 1 take, got %d", len(takes))
	}
	rec = getLiveTakes(t, h, "limit=9999")
	testutil.AssertStatus(t, rec, http.StatusOK)
	if takes = decodeLiveTakes(t, rec); len(takes) != 3 {
		t.Errorf("limit=9999: expected all 3 takes, got %d", len(takes))
	}
}

func TestSportsLiveTakes_EmptyIsNonNullArray(t *testing.T) {
	h, _, _ := setupSportsLiveTest(t)

	rec := getLiveTakes(t, h, "")
	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp struct {
		Data []models.SportsAgentTake `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data == nil {
		t.Error("expected non-null empty data array")
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 takes, got %d", len(resp.Data))
	}
}
