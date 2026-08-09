package sports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/models"
)

func readESPNFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/espn_summary_trimmed.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

// parsedLineups mirrors the compact lineups projection ParseSummary emits.
type parsedLineups struct {
	Home parsedLineupSide `json:"home"`
	Away parsedLineupSide `json:"away"`
}

type parsedLineupSide struct {
	Team      string             `json:"team"`
	Formation string             `json:"formation"`
	Starters  []parsedLineupSlot `json:"starters"`
	Bench     []parsedLineupSlot `json:"bench"`
}

type parsedLineupSlot struct {
	Name   string `json:"name"`
	Jersey string `json:"jersey"`
	Pos    string `json:"pos"`
}

func TestParseSummary(t *testing.T) {
	raw := readESPNFixture(t)

	events, lineups, err := ParseSummary(raw)
	if err != nil {
		t.Fatalf("ParseSummary: %v", err)
	}
	if len(events) != 11 {
		t.Fatalf("expected 11 events, got %d", len(events))
	}

	// Pin the full kind histogram: seq 0 (no play), seq 2 (start-half),
	// seq 5 (foul) and seq 100 ("Second Half ends", no play) are all "play".
	hist := map[string]int{}
	for _, ev := range events {
		hist[ev.Kind]++
	}
	want := map[string]int{"play": 4, "goal": 2, "card": 2, "sub": 1, "ht": 1, "ft": 1}
	if len(hist) != len(want) {
		t.Errorf("kind histogram %v, want %v", hist, want)
	}
	for k, n := range want {
		if hist[k] != n {
			t.Errorf("kind %q: got %d, want %d (histogram %v)", k, hist[k], n, hist)
		}
	}

	last := events[len(events)-1]
	if last.Kind != "ft" || last.Seq != 101 {
		t.Errorf("last event: kind=%q seq=%d, want ft/101", last.Kind, last.Seq)
	}

	if events[0].Minute != nil {
		t.Errorf("events[0].Minute = %v, want nil for empty displayValue", *events[0].Minute)
	}
	if events[3].Minute == nil || *events[3].Minute != "9'" {
		t.Errorf("events[3].Minute = %v, want \"9'\"", events[3].Minute)
	}
	if events[3].Kind != "goal" {
		t.Errorf("events[3].Kind = %q, want goal", events[3].Kind)
	}

	if lineups == nil {
		t.Fatal("expected non-nil lineups when both rosters present")
	}
	var lu parsedLineups
	if err := json.Unmarshal(lineups, &lu); err != nil {
		t.Fatalf("unmarshal lineups: %v", err)
	}
	if lu.Home.Formation != "4-2-3-1" {
		t.Errorf("home formation %q, want 4-2-3-1", lu.Home.Formation)
	}
	if lu.Home.Team != "Mexico" {
		t.Errorf("home team %q, want Mexico", lu.Home.Team)
	}
	if len(lu.Home.Starters) != 1 {
		t.Fatalf("home starters: got %d, want 1", len(lu.Home.Starters))
	}
	if len(lu.Home.Bench) != 1 {
		t.Fatalf("home bench: got %d, want 1", len(lu.Home.Bench))
	}
	b := lu.Home.Bench[0]
	if b.Name != "Raúl Jiménez" || b.Pos != "F" || b.Jersey != "9" {
		t.Errorf("home bench[0] = %+v, want Raúl Jiménez/F/9", b)
	}
}

func TestParseSummaryDuplicateSeqLastWins(t *testing.T) {
	// ESPN payload drift can repeat a sequence number. The repo upserts all
	// events in one statement, and Postgres rejects a batch that touches the
	// same row twice (error 21000), poisoning the whole batch. ParseSummary
	// must dedup with last-wins semantics.
	raw := []byte(`{
		"commentary": [
			{"sequence": 1, "time": {"displayValue": ""}, "text": "Kick off."},
			{"sequence": 7, "time": {"displayValue": "12'"}, "text": "First version of the line."},
			{"sequence": 7, "time": {"displayValue": "12'"}, "text": "Corrected version of the line."}
		],
		"rosters": []
	}`)

	events, _, err := ParseSummary(raw)
	if err != nil {
		t.Fatalf("ParseSummary: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events after dedup, got %d", len(events))
	}
	var seq7 *models.SportsMatchEvent
	for i := range events {
		if events[i].Seq == 7 {
			if seq7 != nil {
				t.Fatal("seq 7 appears more than once after dedup")
			}
			seq7 = &events[i]
		}
	}
	if seq7 == nil {
		t.Fatal("seq 7 missing after dedup")
	}
	if seq7.Body != "Corrected version of the line." {
		t.Errorf("seq 7 body = %q, want the second occurrence (last-wins)", seq7.Body)
	}
}

func TestKindFromPlay(t *testing.T) {
	cases := []struct {
		typ, text, want string
	}{
		{"goal", "Goal! Mexico 1, South Africa 0.", "goal"},
		{"goal---header", "Goal! Mexico 2, South Africa 0.", "goal"},
		{"own-goal", "Own Goal by somebody.", "goal"},
		{"red-card", "Yaya Sithole (South Africa) is shown the red card.", "card"},
		{"yellow-card", "Teboho Mokoena (South Africa) is shown the yellow card.", "card"},
		{"substitution", "Substitution, South Africa.", "sub"},
		{"halftime", "First Half ends, Mexico 1, South Africa 0.", "ht"},
		{"", "Match ends, Mexico 2, South Africa 0.", "ft"},
		{"", "Lineups are announced and players are warming up.", "play"},
		{"foul", "Foul by Teboho Mokoena (South Africa).", "play"},
		{"var---referee-decision-cancelled", "VAR Decision.", "play"},
		{"var---goal-cancelled", "VAR Decision: goal cancelled.", "play"},
		{"var---goal---header-cancelled", "VAR Decision: header goal cancelled.", "play"},
	}
	for _, tc := range cases {
		if got := kindFromPlay(tc.typ, tc.text); got != tc.want {
			t.Errorf("kindFromPlay(%q, %q) = %q, want %q", tc.typ, tc.text, got, tc.want)
		}
	}
}

func TestTeamMatching(t *testing.T) {
	// Real football-data field values for our side. The away name is what
	// football-data.org uses for BIH ("Bosnia and Herzegovina"); ESPN
	// renders the same team as "Bosnia-Herzegovina", which normalizes to
	// "bosniaherzegovina" — NOT equal to or contained in our
	// "bosniaandherzegovina", so only the TLA path catches it.
	m := &models.SportsMatch{
		HomeTeam: "Canada", HomeCode: "CAN",
		AwayTeam: "Bosnia and Herzegovina", AwayCode: "BIH",
	}

	cases := []struct {
		name                                   string
		homeAbbr, homeName, awayAbbr, awayName string
		want                                   bool
	}{
		{"TLA hit both sides", "CAN", "Canada", "BIH", "Bosnia-Herzegovina", true},
		{"name-normalization hit", "XX", "Canada", "YY", "Bosnia and Herzegovina", true},
		{"wrong fixture", "MEX", "Mexico", "RSA", "South Africa", false},
		{"one-side-only match", "CAN", "Canada", "RSA", "South Africa", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchByTeams(m, tc.homeAbbr, tc.homeName, tc.awayAbbr, tc.awayName)
			if got != tc.want {
				t.Errorf("matchByTeams(%q/%q vs %q/%q) = %v, want %v",
					tc.homeAbbr, tc.homeName, tc.awayAbbr, tc.awayName, got, tc.want)
			}
		})
	}
}

func TestScoreboardAndSummaryFetch(t *testing.T) {
	fixture := readESPNFixture(t)

	// The second event is malformed (no competitors): Scoreboard must skip it
	// fail-open rather than error.
	const scoreboardJSON = `{
		"events": [
			{
				"id": "760415",
				"competitions": [
					{
						"competitors": [
							{"homeAway": "home", "team": {"displayName": "Mexico", "abbreviation": "MEX"}},
							{"homeAway": "away", "team": {"displayName": "South Africa", "abbreviation": "RSA"}}
						]
					}
				]
			},
			{
				"id": "760999",
				"competitions": [
					{
						"competitors": []
					}
				]
			}
		]
	}`

	var gotScoreboardQuery, gotSummaryQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/scoreboard":
			gotScoreboardQuery = r.URL.RawQuery
			w.Write([]byte(scoreboardJSON))
		case "/summary":
			gotSummaryQuery = r.URL.RawQuery
			w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewESPNClient()
	c.base = srv.URL

	day := time.Date(2026, 6, 11, 15, 30, 0, 0, time.UTC)
	events, err := c.Scoreboard(context.Background(), day)
	if err != nil {
		t.Fatalf("Scoreboard: %v", err)
	}
	if gotScoreboardQuery != "dates=20260611" {
		t.Errorf("scoreboard query %q, want dates=20260611", gotScoreboardQuery)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 scoreboard event (malformed one skipped), got %d: %+v", len(events), events)
	}
	ev := events[0]
	if ev.ID != 760415 {
		t.Errorf("event id %d, want 760415", ev.ID)
	}
	if ev.HomeName != "Mexico" || ev.HomeAbbr != "MEX" {
		t.Errorf("home = %q/%q, want Mexico/MEX", ev.HomeName, ev.HomeAbbr)
	}
	if ev.AwayName != "South Africa" || ev.AwayAbbr != "RSA" {
		t.Errorf("away = %q/%q, want South Africa/RSA", ev.AwayName, ev.AwayAbbr)
	}

	raw, err := c.SummaryRaw(context.Background(), 760415)
	if err != nil {
		t.Fatalf("SummaryRaw: %v", err)
	}
	if gotSummaryQuery != "event=760415" {
		t.Errorf("summary query %q, want event=760415", gotSummaryQuery)
	}
	if string(raw) != string(fixture) {
		t.Errorf("SummaryRaw returned %d bytes that differ from the fixture (%d bytes)", len(raw), len(fixture))
	}

	// Non-200 responses are errors on both endpoints.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(bad.Close)
	c2 := NewESPNClient()
	c2.base = bad.URL
	if _, err := c2.Scoreboard(context.Background(), day); err == nil {
		t.Error("expected error from Scoreboard on non-200 response")
	}
	if _, err := c2.SummaryRaw(context.Background(), 760415); err == nil {
		t.Error("expected error from SummaryRaw on non-200 response")
	}
}

// fakeEnrichRepo records enrichment persistence calls so Enricher.tick can be
// exercised without a database.
type fakeEnrichRepo struct {
	matches  []models.SportsMatch
	setCalls []setEnrichmentCall
	upserts  []upsertEventsCall
}

type setEnrichmentCall struct {
	matchID     string
	espnEventID int64
	lineups     []byte
}

type upsertEventsCall struct {
	matchID string
	events  []models.SportsMatchEvent
}

func (f *fakeEnrichRepo) MatchesToEnrich(ctx context.Context) ([]models.SportsMatch, error) {
	return f.matches, nil
}

func (f *fakeEnrichRepo) SetEnrichment(ctx context.Context, matchID string, espnEventID int64, lineups []byte) error {
	f.setCalls = append(f.setCalls, setEnrichmentCall{matchID, espnEventID, lineups})
	return nil
}

func (f *fakeEnrichRepo) UpsertEvents(ctx context.Context, matchID string, events []models.SportsMatchEvent) error {
	f.upserts = append(f.upserts, upsertEventsCall{matchID, events})
	return nil
}

// enrichScoreboardJSON is the one-event scoreboard the enricher tests serve:
// ESPN event 760415, Mexico (home) vs South Africa (away) — the fixture match.
const enrichScoreboardJSON = `{
	"events": [
		{
			"id": "760415",
			"competitions": [
				{
					"competitors": [
						{"homeAway": "home", "team": {"displayName": "Mexico", "abbreviation": "MEX"}},
						{"homeAway": "away", "team": {"displayName": "South Africa", "abbreviation": "RSA"}}
					]
				}
			]
		}
	]
}`

func TestEnricherTick(t *testing.T) {
	fixture := readESPNFixture(t)

	// ESPN buckets scoreboard days by US Eastern, not UTC, so discovery
	// fetches both the kickoff's UTC day and the day before. Model the real
	// failure mode (KOR/CZE kicked off 2026-06-12T02:00Z but sat under
	// dates=20260611): serve the event ONLY under kickoff day - 1 and an
	// empty list under the kickoff day itself.
	kickoff := time.Date(2026, 6, 12, 2, 0, 0, 0, time.UTC)
	kickoffDay := kickoff.Format("20060102")                // 20260612
	prevDay := kickoff.AddDate(0, 0, -1).Format("20060102") // 20260611

	var scoreboardReqs int
	scoreboardDays := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/scoreboard":
			scoreboardReqs++
			dates := r.URL.Query().Get("dates")
			scoreboardDays[dates]++
			switch dates {
			case prevDay:
				w.Write([]byte(enrichScoreboardJSON))
			case kickoffDay:
				w.Write([]byte(`{"events": []}`))
			default:
				t.Errorf("unexpected scoreboard dates %q", dates)
				w.Write([]byte(`{"events": []}`))
			}
		case "/summary":
			w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	repo := &fakeEnrichRepo{matches: []models.SportsMatch{{
		ID:       "m1",
		HomeTeam: "Mexico", HomeCode: "MEX",
		AwayTeam: "South Africa", AwayCode: "RSA",
		KickoffUTC: kickoff,
	}}}

	e := NewEnricher(&ESPNClient{base: srv.URL, client: srv.Client()}, repo)

	// First tick: no ESPN id on the match → scoreboard discovery across both
	// days (the event lives only under day-1), then summary fetch, then
	// persistence of both enrichment and events.
	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if scoreboardReqs != 2 {
		t.Errorf("scoreboard requests = %d, want 2 (kickoff day + day-1)", scoreboardReqs)
	}
	if scoreboardDays[kickoffDay] != 1 || scoreboardDays[prevDay] != 1 {
		t.Errorf("scoreboard days fetched = %v, want one fetch each for %s and %s",
			scoreboardDays, kickoffDay, prevDay)
	}
	if len(repo.setCalls) != 1 {
		t.Fatalf("SetEnrichment calls = %d, want 1", len(repo.setCalls))
	}
	sc := repo.setCalls[0]
	if sc.matchID != "m1" || sc.espnEventID != 760415 {
		t.Errorf("SetEnrichment(%q, %d), want (m1, 760415)", sc.matchID, sc.espnEventID)
	}
	if sc.lineups == nil {
		t.Error("SetEnrichment lineups = nil, want non-nil (fixture has both rosters)")
	}
	if len(repo.upserts) != 1 {
		t.Fatalf("UpsertEvents calls = %d, want 1", len(repo.upserts))
	}
	if up := repo.upserts[0]; up.matchID != "m1" || len(up.events) != 11 {
		t.Errorf("UpsertEvents(%q, %d events), want (m1, 11 events)", up.matchID, len(up.events))
	}

	// Second tick: the match now carries its ESPN id → no scoreboard
	// discovery (request count unchanged), but the summary is refreshed
	// and events upserted again (idempotent).
	id := int64(760415)
	repo.matches[0].ESPNEventID = &id
	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if scoreboardReqs != 2 {
		t.Errorf("scoreboard requests after second tick = %d, want still 2 (id known)", scoreboardReqs)
	}
	if len(repo.upserts) != 2 {
		t.Errorf("UpsertEvents calls after second tick = %d, want 2 (idempotent refresh)", len(repo.upserts))
	}
}

func TestEnricherTickFailOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	// Scoreboard 500 during id discovery: the tick logs, skips the match,
	// and returns nil — never an error that would abort the loop.
	repo := &fakeEnrichRepo{matches: []models.SportsMatch{{
		ID:       "m1",
		HomeTeam: "Mexico", HomeCode: "MEX",
		AwayTeam: "South Africa", AwayCode: "RSA",
		KickoffUTC: time.Now().UTC(),
	}}}
	e := NewEnricher(&ESPNClient{base: srv.URL, client: srv.Client()}, repo)
	if err := e.tick(context.Background()); err != nil {
		t.Fatalf("tick with scoreboard 500 = %v, want nil (fail-open)", err)
	}
	if len(repo.setCalls) != 0 {
		t.Errorf("SetEnrichment calls = %d, want 0 after scoreboard failure", len(repo.setCalls))
	}
	if len(repo.upserts) != 0 {
		t.Errorf("UpsertEvents calls = %d, want 0 after scoreboard failure", len(repo.upserts))
	}

	// Summary 500 for a match with a known ESPN id: same fail-open skip.
	id := int64(760415)
	repo2 := &fakeEnrichRepo{matches: []models.SportsMatch{{
		ID:       "m2",
		HomeTeam: "Mexico", HomeCode: "MEX",
		AwayTeam: "South Africa", AwayCode: "RSA",
		KickoffUTC:  time.Now().UTC(),
		ESPNEventID: &id,
	}}}
	e2 := NewEnricher(&ESPNClient{base: srv.URL, client: srv.Client()}, repo2)
	if err := e2.tick(context.Background()); err != nil {
		t.Fatalf("tick with summary 500 = %v, want nil (fail-open)", err)
	}
	if len(repo2.setCalls) != 0 {
		t.Errorf("SetEnrichment calls = %d, want 0 after summary failure", len(repo2.setCalls))
	}
	if len(repo2.upserts) != 0 {
		t.Errorf("UpsertEvents calls = %d, want 0 after summary failure", len(repo2.upserts))
	}
}
