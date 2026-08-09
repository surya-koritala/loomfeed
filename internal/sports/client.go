// Package sports integrates World Cup data: the football-data.org
// schedule/score client + poller, an ESPN enrichment client
// (timelines/lineups), and LLM jobs built on them.
package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RoamXAI/loomfeed/internal/models"
)

// defaultBase is the football-data.org v4 API root.
const defaultBase = "https://api.football-data.org/v4"

// worldCupCompetitionID is football-data.org's id for the FIFA World Cup.
const worldCupCompetitionID = "2000"

// Client fetches World Cup match data from football-data.org.
//
// It deliberately uses a plain http.Client rather than safehttp: the base URL
// is fixed operator configuration (never user-supplied, so SSRF does not
// apply), and safehttp's dial guard blocks the loopback addresses that
// httptest servers bind to.
type Client struct {
	apiKey string
	base   string // overridden in tests (white-box, same package)
	http   *http.Client
}

// NewClient creates a football-data.org client. apiKey may be "" — the free
// tier allows unauthenticated requests at a lower rate limit.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		base:   defaultBase,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// fdTeam is one side of a football-data.org match.
type fdTeam struct {
	Name  string `json:"name"`
	TLA   string `json:"tla"`
	Crest string `json:"crest"`
}

// fdScore holds a score pair; fields are null until the match starts.
type fdScore struct {
	Home *int `json:"home"`
	Away *int `json:"away"`
}

// fdMatch is the subset of the football-data.org v4 match shape we consume.
type fdMatch struct {
	ID       int64     `json:"id"`
	UTCDate  time.Time `json:"utcDate"`
	Status   string    `json:"status"`
	Stage    string    `json:"stage"`
	Group    string    `json:"group"` // null outside the group stage
	Venue    string    `json:"venue"`
	HomeTeam fdTeam    `json:"homeTeam"`
	AwayTeam fdTeam    `json:"awayTeam"`
	Score    struct {
		FullTime fdScore `json:"fullTime"`
	} `json:"score"`
}

// fdMatchesResponse is the envelope of GET /competitions/{id}/matches.
type fdMatchesResponse struct {
	Matches []fdMatch `json:"matches"`
}

// FetchWorldCupMatches GETs all World Cup matches and maps them to
// models.SportsMatch with Competition "wc2026". A non-200 response is an
// error that includes the status code.
func (c *Client) FetchWorldCupMatches(ctx context.Context) ([]models.SportsMatch, error) {
	url := c.base + "/competitions/" + worldCupCompetitionID + "/matches"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build football-data request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("X-Auth-Token", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch football-data matches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("football-data: unexpected status %d", resp.StatusCode)
	}

	var payload fdMatchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode football-data matches: %w", err)
	}

	matches := make([]models.SportsMatch, 0, len(payload.Matches))
	for _, fm := range payload.Matches {
		matches = append(matches, models.SportsMatch{
			ExtID:       fm.ID,
			Competition: "wc2026",
			Stage:       fm.Stage,
			GroupName:   fm.Group,
			HomeTeam:    fm.HomeTeam.Name,
			HomeCode:    fm.HomeTeam.TLA,
			HomeCrest:   fm.HomeTeam.Crest,
			AwayTeam:    fm.AwayTeam.Name,
			AwayCode:    fm.AwayTeam.TLA,
			AwayCrest:   fm.AwayTeam.Crest,
			KickoffUTC:  fm.UTCDate,
			Status:      fm.Status,
			HomeScore:   fm.Score.FullTime.Home,
			AwayScore:   fm.Score.FullTime.Away,
			Venue:       fm.Venue,
		})
	}
	return matches, nil
}
