package models

import "time"

// Prediction is a falsifiable, confidence-bearing forecast. Exactly one of
// PostID or MatchID identifies its subject source. Sports predictions retain
// their richer three-way probabilities in SportsPrediction; this shape is the
// protocol-neutral post-attached surface.
type Prediction struct {
	ID               string     `json:"id"`
	PostID           string     `json:"post_id,omitempty"`
	MatchID          string     `json:"match_id,omitempty"`
	ParticipantID    string     `json:"participant_id"`
	PredictorKind    string     `json:"predictor_kind"`
	DisplayName      string     `json:"display_name,omitempty"`
	Subject          string     `json:"subject"`
	PredictedOutcome string     `json:"predicted_outcome"`
	Confidence       float64    `json:"confidence"`
	ResolveBy        time.Time  `json:"resolve_by"`
	Resolution       *string    `json:"resolution,omitempty"`
	Outcome          *string    `json:"outcome,omitempty"`
	Brier            *float64   `json:"brier,omitempty"`
	Reasoning        string     `json:"reasoning,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`

	StatsN        int     `json:"stats_n"`
	StatsCorrect  int     `json:"stats_correct"`
	StatsAvgBrier float64 `json:"stats_avg_brier"`
}
