package sports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestdataServer serves the trimmed football-data.org payload and records
// the last request's path and whether/what X-Auth-Token was sent.
func newTestdataServer(t *testing.T) (srv *httptest.Server, gotPath *string, gotToken *string, tokenSent *bool) {
	t.Helper()
	gotPath, gotToken, tokenSent = new(string), new(string), new(bool)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		_, *tokenSent = r.Header[http.CanonicalHeaderKey("X-Auth-Token")]
		*gotToken = r.Header.Get("X-Auth-Token")
		http.ServeFile(w, r, "testdata/wc_matches.json")
	}))
	t.Cleanup(srv.Close)
	return srv, gotPath, gotToken, tokenSent
}

func TestFetchWorldCupMatches_MapsPayload(t *testing.T) {
	srv, gotPath, gotToken, _ := newTestdataServer(t)

	c := NewClient("test-key")
	c.base = srv.URL
	matches, err := c.FetchWorldCupMatches(context.Background())
	if err != nil {
		t.Fatalf("FetchWorldCupMatches: %v", err)
	}

	if *gotPath != "/competitions/2000/matches" {
		t.Errorf("expected path /competitions/2000/matches, got %q", *gotPath)
	}
	if *gotToken != "test-key" {
		t.Errorf("expected X-Auth-Token test-key, got %q", *gotToken)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}

	// Match 1: TIMED upcoming — full field mapping, nil scores.
	m := matches[0]
	if m.ExtID != 524289 {
		t.Errorf("expected ext_id 524289, got %d", m.ExtID)
	}
	if m.Competition != "wc2026" {
		t.Errorf("expected competition wc2026, got %q", m.Competition)
	}
	if m.Stage != "GROUP_STAGE" {
		t.Errorf("expected stage GROUP_STAGE, got %q", m.Stage)
	}
	if m.GroupName != "Group A" {
		t.Errorf("expected group_name 'Group A', got %q", m.GroupName)
	}
	if m.Venue != "Estadio Azteca" {
		t.Errorf("expected venue 'Estadio Azteca', got %q", m.Venue)
	}
	if m.HomeTeam != "Mexico" || m.HomeCode != "MEX" {
		t.Errorf("expected home Mexico/MEX, got %q/%q", m.HomeTeam, m.HomeCode)
	}
	if m.HomeCrest != "https://crests.football-data.org/769.png" {
		t.Errorf("unexpected home crest %q", m.HomeCrest)
	}
	if m.AwayTeam != "Poland" || m.AwayCode != "POL" {
		t.Errorf("expected away Poland/POL, got %q/%q", m.AwayTeam, m.AwayCode)
	}
	if m.AwayCrest != "https://crests.football-data.org/794.png" {
		t.Errorf("unexpected away crest %q", m.AwayCrest)
	}
	wantKickoff := time.Date(2026, 6, 13, 20, 0, 0, 0, time.UTC)
	if !m.KickoffUTC.Equal(wantKickoff) {
		t.Errorf("expected kickoff %v, got %v", wantKickoff, m.KickoffUTC)
	}
	if m.Status != "TIMED" {
		t.Errorf("expected status TIMED, got %q", m.Status)
	}
	if m.HomeScore != nil || m.AwayScore != nil {
		t.Errorf("expected nil scores for TIMED match, got %v/%v", m.HomeScore, m.AwayScore)
	}

	// Match 2: IN_PLAY — running score 1-0.
	m = matches[1]
	if m.ExtID != 524290 || m.Status != "IN_PLAY" {
		t.Errorf("expected 524290 IN_PLAY, got %d %q", m.ExtID, m.Status)
	}
	if m.HomeScore == nil || *m.HomeScore != 1 {
		t.Errorf("expected home_score 1, got %v", m.HomeScore)
	}
	if m.AwayScore == nil || *m.AwayScore != 0 {
		t.Errorf("expected away_score 0, got %v", m.AwayScore)
	}

	// Match 3: FINISHED — final score 2-1.
	m = matches[2]
	if m.ExtID != 524291 || m.Status != "FINISHED" {
		t.Errorf("expected 524291 FINISHED, got %d %q", m.ExtID, m.Status)
	}
	if m.HomeScore == nil || *m.HomeScore != 2 {
		t.Errorf("expected home_score 2, got %v", m.HomeScore)
	}
	if m.AwayScore == nil || *m.AwayScore != 1 {
		t.Errorf("expected away_score 1, got %v", m.AwayScore)
	}
}

func TestFetchWorldCupMatches_OmitsHeaderWhenNoKey(t *testing.T) {
	srv, _, _, tokenSent := newTestdataServer(t)

	c := NewClient("")
	c.base = srv.URL
	if _, err := c.FetchWorldCupMatches(context.Background()); err != nil {
		t.Fatalf("FetchWorldCupMatches: %v", err)
	}
	if *tokenSent {
		t.Error("expected X-Auth-Token header to be omitted when apiKey is empty")
	}
}

func TestFetchWorldCupMatches_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("test-key")
	c.base = srv.URL
	_, err := c.FetchWorldCupMatches(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to include status code 429, got %q", err.Error())
	}
}
