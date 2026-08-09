package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/RoamXAI/loomfeed/internal/models"
)

// espnDefaultBase is the root of ESPN's unofficial site API for the FIFA
// World Cup. Free, keyless — and undocumented, so it can change or block at
// any time. EVERYTHING built on it must be fail-open enrichment: errors are
// logged and skipped; the page degrades to the football-data v1 experience.
// Page traffic never reaches ESPN — the enricher polls and stores.
const espnDefaultBase = "https://site.api.espn.com/apis/site/v2/sports/soccer/fifa.world"

// ESPNClient fetches enrichment data (commentary, lineups) from ESPN's
// unofficial site API.
//
// It deliberately uses a plain http.Client rather than safehttp: the base URL
// is fixed operator configuration (never user-supplied, so SSRF does not
// apply), and safehttp's dial guard blocks the loopback addresses that
// httptest servers bind to.
type ESPNClient struct {
	base   string // overridden in tests (white-box, same package)
	client *http.Client
}

// NewESPNClient creates an ESPN site-API client.
func NewESPNClient() *ESPNClient {
	return &ESPNClient{
		base:   espnDefaultBase,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// espnTeam is the team subobject shared by scoreboard competitors and
// summary rosters.
type espnTeam struct {
	DisplayName  string `json:"displayName"`
	Abbreviation string `json:"abbreviation"`
}

// espnCompetitor is one side of a scoreboard event's competition.
type espnCompetitor struct {
	HomeAway string   `json:"homeAway"`
	Team     espnTeam `json:"team"`
}

// espnEvent is one scoreboard event. Note ESPN serves the id as a string.
type espnEvent struct {
	ID           string `json:"id"`
	Competitions []struct {
		Competitors []espnCompetitor `json:"competitors"`
	} `json:"competitions"`
}

// espnScoreboard is the envelope of GET {base}/scoreboard.
type espnScoreboard struct {
	Events []espnEvent `json:"events"`
}

// espnCommentaryLine is one summary commentary entry. Play is a pointer so
// its absence is detectable — pre-match lines and the final "Match ends, …"
// line carry no play object.
type espnCommentaryLine struct {
	Sequence int `json:"sequence"`
	Time     struct {
		DisplayValue string `json:"displayValue"`
	} `json:"time"`
	Text string `json:"text"`
	Play *struct {
		Type struct {
			Type string `json:"type"`
		} `json:"type"`
	} `json:"play"`
}

// espnRoster is one side's lineup in the summary payload.
type espnRoster struct {
	HomeAway  string   `json:"homeAway"`
	Formation string   `json:"formation"`
	Team      espnTeam `json:"team"`
	Roster    []struct {
		Starter  bool   `json:"starter"`
		Jersey   string `json:"jersey"`
		Position struct {
			Abbreviation string `json:"abbreviation"`
		} `json:"position"`
		Athlete struct {
			DisplayName string `json:"displayName"`
		} `json:"athlete"`
	} `json:"roster"`
}

// espnSummary is the subset of GET {base}/summary we consume.
type espnSummary struct {
	Commentary []espnCommentaryLine `json:"commentary"`
	Rosters    []espnRoster         `json:"rosters"`
}

// ESPNScoreboardEvent is one parsed scoreboard event: the ESPN event ID plus
// both sides' names and abbreviations, flattened from the wire shape.
type ESPNScoreboardEvent struct {
	ID                 int64
	HomeName, HomeAbbr string
	AwayName, AwayAbbr string
}

// Scoreboard GETs the World Cup scoreboard for a UTC day and returns the
// parsed events. A non-200 response is an error that includes the status
// code. Malformed events — missing competitions/competitors or an
// unparseable string ID — are skipped fail-open, not errors: this whole
// surface is best-effort enrichment.
func (c *ESPNClient) Scoreboard(ctx context.Context, day time.Time) ([]ESPNScoreboardEvent, error) {
	url := c.base + "/scoreboard?dates=" + day.UTC().Format("20060102")
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var sb espnScoreboard
	if err := json.Unmarshal(body, &sb); err != nil {
		return nil, fmt.Errorf("decode espn scoreboard: %w", err)
	}

	events := make([]ESPNScoreboardEvent, 0, len(sb.Events))
	for _, we := range sb.Events {
		id, err := strconv.ParseInt(we.ID, 10, 64)
		if err != nil || len(we.Competitions) == 0 {
			continue
		}
		out := ESPNScoreboardEvent{ID: id}
		var haveHome, haveAway bool
		for _, comp := range we.Competitions[0].Competitors {
			switch comp.HomeAway {
			case "home":
				out.HomeName, out.HomeAbbr = comp.Team.DisplayName, comp.Team.Abbreviation
				haveHome = true
			case "away":
				out.AwayName, out.AwayAbbr = comp.Team.DisplayName, comp.Team.Abbreviation
				haveAway = true
			}
		}
		if !haveHome || !haveAway {
			continue
		}
		events = append(events, out)
	}
	return events, nil
}

// SummaryRaw GETs a match summary and returns the raw bytes. Parsing is
// separate (ParseSummary) so tests can parse fixtures without HTTP.
func (c *ESPNClient) SummaryRaw(ctx context.Context, eventID int64) ([]byte, error) {
	url := c.base + "/summary?event=" + strconv.FormatInt(eventID, 10)
	return c.get(ctx, url)
}

// espnMaxBody bounds ESPN response bodies. Real summaries measure ~417KB, so
// 4MB is generous headroom, not a truncation point we expect to hit.
const espnMaxBody = 4 << 20

// get performs a GET with a bounded body read. A body larger than
// espnMaxBody is an error, not a silent truncation.
func (c *ESPNClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build espn request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch espn: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("espn: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, espnMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read espn body: %w", err)
	}
	if len(body) > espnMaxBody {
		return nil, fmt.Errorf("espn: body exceeds 4MB limit")
	}
	return body, nil
}

// kindFromPlay classifies a commentary line into our event kinds from the
// ESPN play type (and the text, for lines with no play object). Goal types
// come in prefixed/suffixed forms ("goal", "goal---header", "own-goal").
func kindFromPlay(typ, text string) string {
	// VAR types arrive under "var---..." and can embed "goal" for cancelled
	// goals ("var---goal-cancelled"); they must not classify as goals.
	if strings.HasPrefix(typ, "var") {
		return "play"
	}
	switch {
	case strings.HasPrefix(typ, "goal") || strings.Contains(typ, "-goal"):
		return "goal"
	case typ == "red-card" || typ == "yellow-card":
		return "card"
	case typ == "substitution":
		return "sub"
	case typ == "halftime":
		return "ht"
	case typ == "" && strings.HasPrefix(text, "Match ends"):
		return "ft"
	default:
		return "play"
	}
}

// lineupSlot is one player in the compact lineups projection.
type lineupSlot struct {
	Name   string `json:"name"`
	Jersey string `json:"jersey"`
	Pos    string `json:"pos"`
}

// lineupSide is one team's half of the compact lineups projection.
type lineupSide struct {
	Team      string       `json:"team"`
	Formation string       `json:"formation"`
	Starters  []lineupSlot `json:"starters"`
	Bench     []lineupSlot `json:"bench"`
}

// ParseSummary maps an ESPN summary payload to timeline events plus a
// compact lineups JSON projection. Side/Player are left nil — the event
// body text carries the names; the schema columns exist for a future
// structured pass. Lineups are nil (not an error) unless both sides
// are present in the payload.
func ParseSummary(raw []byte) ([]models.SportsMatchEvent, []byte, error) {
	var s espnSummary
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, nil, fmt.Errorf("decode espn summary: %w", err)
	}

	// Dedup repeated sequence numbers (ESPN payload drift) with last-wins
	// semantics: the repo upserts all events in a single statement, and
	// Postgres rejects a batch that updates the same row twice (error 21000),
	// which would poison the whole batch persistently.
	events := make([]models.SportsMatchEvent, 0, len(s.Commentary))
	bySeq := make(map[int]int, len(s.Commentary)) // seq → index in events
	for _, line := range s.Commentary {
		typ := ""
		if line.Play != nil {
			typ = line.Play.Type.Type
		}
		ev := models.SportsMatchEvent{
			Seq:  line.Sequence,
			Kind: kindFromPlay(typ, line.Text),
			Body: line.Text,
		}
		if dv := line.Time.DisplayValue; dv != "" {
			ev.Minute = &dv
		}
		if i, seen := bySeq[line.Sequence]; seen {
			events[i] = ev
			continue
		}
		bySeq[line.Sequence] = len(events)
		events = append(events, ev)
	}

	var home, away *lineupSide
	for _, r := range s.Rosters {
		side := lineupSide{
			Team:      r.Team.DisplayName,
			Formation: r.Formation,
			Starters:  []lineupSlot{},
			Bench:     []lineupSlot{},
		}
		for _, p := range r.Roster {
			slot := lineupSlot{Name: p.Athlete.DisplayName, Jersey: p.Jersey, Pos: p.Position.Abbreviation}
			if p.Starter {
				side.Starters = append(side.Starters, slot)
			} else {
				side.Bench = append(side.Bench, slot)
			}
		}
		switch r.HomeAway {
		case "home":
			home = &side
		case "away":
			away = &side
		}
	}

	var lineups []byte
	if home != nil && away != nil {
		b, err := json.Marshal(map[string]*lineupSide{"home": home, "away": away})
		if err != nil {
			return nil, nil, fmt.Errorf("marshal lineups: %w", err)
		}
		lineups = b
	}
	return events, lineups, nil
}

// normTeam normalizes a team name for fuzzy comparison: lowercase, letters
// only ("South Africa" → "southafrica").
func normTeam(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// teamMatches reports whether an ESPN competitor refers to our team: TLA
// match, normalized name equality, or normalized containment either way.
// Empty values never match. Containment is deliberately loose ("Korea" ⊂
// "South Korea") to absorb cross-provider name variants; it is safe because
// matchByTeams requires BOTH sides to match within a single day's scoreboard,
// so a one-sided false positive cannot pair a fixture on its own.
func teamMatches(ourName, ourCode, espnAbbr, espnName string) bool {
	if ourCode != "" && espnAbbr != "" && strings.EqualFold(ourCode, espnAbbr) {
		return true
	}
	on, en := normTeam(ourName), normTeam(espnName)
	if on == "" || en == "" {
		return false
	}
	return on == en || strings.Contains(on, en) || strings.Contains(en, on)
}

// matchByTeams reports whether an ESPN scoreboard event's sides line up with
// one of our matches: home must match home AND away must match away.
func matchByTeams(m *models.SportsMatch, homeAbbr, homeName, awayAbbr, awayName string) bool {
	return teamMatches(m.HomeTeam, m.HomeCode, homeAbbr, homeName) &&
		teamMatches(m.AwayTeam, m.AwayCode, awayAbbr, awayName)
}

// enricherRepo is the narrow persistence surface the Enricher needs.
// *repository.SportsRepo satisfies it; tests substitute an in-memory fake.
type enricherRepo interface {
	MatchesToEnrich(ctx context.Context) ([]models.SportsMatch, error)
	SetEnrichment(ctx context.Context, matchID string, espnEventID int64, lineups []byte) error
	UpsertEvents(ctx context.Context, matchID string, events []models.SportsMatchEvent) error
}

// Enricher polls ESPN for live/imminent matches and stores timeline
// events + lineups. Strictly fail-open: any error is logged and the
// tick moves on; no retries, no state besides what's in the DB.
type Enricher struct {
	espn *ESPNClient
	repo enricherRepo
}

// NewEnricher creates an Enricher.
func NewEnricher(espn *ESPNClient, repo enricherRepo) *Enricher {
	return &Enricher{espn: espn, repo: repo}
}

// enrichInterval is the fixed enrichment cadence. MatchesToEnrich already
// narrows the work to live/imminent matches, so an empty tick is one cheap
// DB query and zero ESPN calls.
const enrichInterval = 90 * time.Second

// Run ticks immediately, then loops until ctx is done, mirroring the
// football-data poller's timer pattern (poller.go).
func (e *Enricher) Run(ctx context.Context) {
	for {
		if err := e.tick(ctx); err != nil {
			slog.Warn("sports enrich: tick failed", "error", err)
		}

		timer := time.NewTimer(enrichInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// tick enriches every match MatchesToEnrich returns: discovers the ESPN
// event id from the scoreboard when unknown, fetches and parses the
// summary, then persists enrichment (id + lineups) and timeline events.
// Per-match errors are logged and skipped — one bad match never aborts
// the others; only the initial DB read is a tick-level error.
//
// ESPN's scoreboard buckets matches by US Eastern time, NOT UTC: South
// Korea vs Czechia kicked off 2026-06-12T02:00:00Z yet appears under
// dates=20260611 (observed 2026-06-12). Formatting the kickoff's UTC day
// alone would therefore miss every match kicking off in the first ~4 UTC
// hours of a day — common in this North-American World Cup. Rather than
// converting timezones (tzdata availability in the container is a
// deployment variable), discovery fetches BOTH the kickoff's UTC day and
// the day before and searches the union. Both team pairs must line up, so
// the extra day cannot introduce a false positive.
//
// That union is fetched lazily, at most once per tick, for the FIRST
// id-less match's kickoff. Matches in one tick may still span scoreboard
// days the union doesn't cover; that's acceptable: such a match simply
// finds no scoreboard entry (no false positive — both team pairs must
// line up) and is picked up on a later tick, once the first day's matches
// have their ids and it becomes the first id-less match.
func (e *Enricher) tick(ctx context.Context) error {
	matches, err := e.repo.MatchesToEnrich(ctx)
	if err != nil {
		return err
	}

	var sb []ESPNScoreboardEvent
	var sbFetched, sbErr bool
	for i := range matches {
		m := &matches[i]
		id := int64(0)
		if m.ESPNEventID != nil {
			id = *m.ESPNEventID
		}
		if id == 0 {
			if !sbFetched && !sbErr {
				sbFetched = true
				sb, sbErr = e.scoreboardUnion(ctx, m.KickoffUTC)
			}
			if sbErr {
				continue
			}
			id = findESPNEvent(sb, m)
			if id == 0 {
				slog.Warn("sports enrich: no espn event matched", "match", m.ID)
				continue
			}
		}

		raw, err := e.espn.SummaryRaw(ctx, id)
		if err != nil {
			slog.Warn("sports enrich: summary failed", "match", m.ID, "error", err)
			continue
		}
		events, lineups, err := ParseSummary(raw)
		if err != nil {
			slog.Warn("sports enrich: parse failed", "match", m.ID, "error", err)
			continue
		}
		if err := e.repo.SetEnrichment(ctx, m.ID, id, lineups); err != nil {
			slog.Warn("sports enrich: persist enrichment failed", "match", m.ID, "error", err)
			continue
		}
		if len(events) > 0 {
			if err := e.repo.UpsertEvents(ctx, m.ID, events); err != nil {
				slog.Warn("sports enrich: upsert events failed", "match", m.ID, "error", err)
			}
		}
	}
	return nil
}

// scoreboardUnion fetches the ESPN scoreboard for the kickoff's UTC day AND
// the day before, returning the combined events (see tick for why: ESPN
// buckets by US Eastern, not UTC). failed is true only when BOTH fetches
// fail; a single failure is logged and the other day's events are used —
// partial discovery beats none.
func (e *Enricher) scoreboardUnion(ctx context.Context, kickoff time.Time) (events []ESPNScoreboardEvent, failed bool) {
	failures := 0
	for _, day := range []time.Time{kickoff, kickoff.AddDate(0, 0, -1)} {
		evs, err := e.espn.Scoreboard(ctx, day)
		if err != nil {
			slog.Warn("sports enrich: scoreboard failed",
				"day", day.UTC().Format("20060102"), "error", err)
			failures++
			continue
		}
		events = append(events, evs...)
	}
	return events, failures == 2
}

// findESPNEvent returns the scoreboard event id whose sides match ours, or 0.
func findESPNEvent(events []ESPNScoreboardEvent, m *models.SportsMatch) int64 {
	for _, ev := range events {
		if matchByTeams(m, ev.HomeAbbr, ev.HomeName, ev.AwayAbbr, ev.AwayName) {
			return ev.ID
		}
	}
	return 0
}
