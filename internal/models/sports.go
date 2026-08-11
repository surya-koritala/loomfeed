package models

import (
	"encoding/json"
	"time"
)

// SportsMatch is a World Cup match tracked for predictions.
type SportsMatch struct {
	ID          string     `json:"id"`
	ExtID       int64      `json:"ext_id"`
	Competition string     `json:"competition"`
	Stage       string     `json:"stage"`
	GroupName   string     `json:"group_name"`
	HomeTeam    string     `json:"home_team"`
	HomeCode    string     `json:"home_code"`
	HomeCrest   string     `json:"home_crest"`
	AwayTeam    string     `json:"away_team"`
	AwayCode    string     `json:"away_code"`
	AwayCrest   string     `json:"away_crest"`
	KickoffUTC  time.Time  `json:"kickoff_utc"`
	Status      string     `json:"status"`
	HomeScore   *int       `json:"home_score"`
	AwayScore   *int       `json:"away_score"`
	Venue       string     `json:"venue"`
	SettledAt   *time.Time `json:"-"`

	ESPNEventID *int64          `json:"espn_event_id,omitempty"`
	Lineups     json.RawMessage `json:"lineups,omitempty"`

	// PredictionCount is the number of agent predictions on the match,
	// computed by a correlated subquery in sportsMatchColumns.
	PredictionCount int `json:"prediction_count"`
}

// SportsPrediction is a participant's pre-kickoff call on a match.
// Agents submit full win/draw/away probabilities; humans submit a single pick.
type SportsPrediction struct {
	ID            string    `json:"id"`
	MatchID       string    `json:"match_id"`
	ParticipantID string    `json:"participant_id"`
	PredictorKind string    `json:"predictor_kind"`
	DisplayName   string    `json:"display_name"` // joined from participants
	HomeProb      *float64  `json:"home_prob,omitempty"`
	DrawProb      *float64  `json:"draw_prob,omitempty"`
	AwayProb      *float64  `json:"away_prob,omitempty"`
	Pick          string    `json:"pick"`
	Reasoning     string    `json:"reasoning,omitempty"`
	Outcome       *string   `json:"outcome"`
	Brier         *float64  `json:"brier,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	// Track record joined from prediction_stats (zero until ≥1 settled).
	StatsN       int      `json:"stats_n"`
	StatsCorrect int      `json:"stats_correct"`
	StatsBrier   *float64 `json:"stats_avg_brier,omitempty"`
}

// DeriveSportsPick returns the argmax pick for a probability triple with a
// deterministic tiebreak: home > draw > away. Shared by the prediction API
// handler and the sports auto-predictor so both derive identical picks.
func DeriveSportsPick(home, draw, away float64) string {
	switch {
	case home >= draw && home >= away:
		return "home"
	case draw >= away:
		return "draw"
	default:
		return "away"
	}
}

// SportsLeaderboardRow is one entry in the prediction accuracy leaderboard.
type SportsLeaderboardRow struct {
	ParticipantID string   `json:"participant_id"`
	DisplayName   string   `json:"display_name"`
	PredictorKind string   `json:"predictor_kind"`
	N             int      `json:"n"`
	Correct       int      `json:"correct"`
	Accuracy      float64  `json:"accuracy"`
	AvgBrier      *float64 `json:"avg_brier,omitempty"`
	Streak        int      `json:"streak"`
}

// SportsMatchEvent is one entry in a match's enrichment timeline —
// either an ESPN play-by-play line (kind "play") or a key moment
// (goal/card/sub/ht/ft). seq is ESPN's commentary sequence, which is
// stable across re-polls (UNIQUE(match_id, seq) makes upserts idempotent).
type SportsMatchEvent struct {
	ID        string    `json:"id"`
	MatchID   string    `json:"match_id"`
	Seq       int       `json:"seq"`
	Minute    *string   `json:"minute,omitempty"`
	Kind      string    `json:"kind"`
	Side      *string   `json:"side,omitempty"`
	Player    *string   `json:"player,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// SportsAgentTake is an in-house agent's short live reaction to a match
// event. EventSeq ties it to the timeline entry it reacts to (nil =
// pre-match take).
type SportsAgentTake struct {
	ID            string    `json:"id"`
	MatchID       string    `json:"match_id"`
	ParticipantID string    `json:"participant_id"`
	EventSeq      *int      `json:"event_seq,omitempty"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
	// Joined for display.
	DisplayName string  `json:"display_name,omitempty"`
	Pick        *string `json:"pick,omitempty"`
	Outcome     *string `json:"outcome,omitempty"`
}

// SportsTimelineItem is one row of the merged timeline stream.
// Exactly one of Event/Take is set; Kind discriminates ("event"|"take").
type SportsTimelineItem struct {
	Kind  string            `json:"kind"`
	Event *SportsMatchEvent `json:"event,omitempty"`
	Take  *SportsAgentTake  `json:"take,omitempty"`
}

// SportsStandingRow is one team's group-stage line, computed from our
// own FINISHED results (never from ESPN).
type SportsStandingRow struct {
	GroupName string `json:"group_name"`
	Team      string `json:"team"`
	Code      string `json:"code"`
	Played    int    `json:"played"`
	Won       int    `json:"won"`
	Drawn     int    `json:"drawn"`
	Lost      int    `json:"lost"`
	GF        int    `json:"gf"`
	GA        int    `json:"ga"`
	GD        int    `json:"gd"`
	Points    int    `json:"points"`
}
