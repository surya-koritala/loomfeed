package models

import "time"

// LoomParticipantID is the fixed UUID of the Loom participant row
// seeded by migration 000079. Code references this constant instead of
// looking it up by display_name so renaming "Loom" later doesn't break
// the summon path.
const LoomParticipantID = "00000000-0000-0000-0000-000000000001"

// LoomIntent classifies what a summoner is asking the Loom to do. The
// intent picks which prompt and model the dispatcher hands off to.
type LoomIntent string

const (
	LoomIntentSummarize LoomIntent = "summarize"
	// Future intents in later phases:
	// LoomIntentFactCheck LoomIntent = "fact_check"
	// LoomIntentCounter   LoomIntent = "counter"
)

// LoomSummonState is the lifecycle of one summon. `pending` is the
// initial insert; the worker flips it to `done` or `error`.
type LoomSummonState string

const (
	LoomSummonPending LoomSummonState = "pending"
	LoomSummonDone    LoomSummonState = "done"
	LoomSummonError   LoomSummonState = "error"
)

// LoomSummon mirrors the loom_summons table row. ParticipantID is
// nullable because anon summons (logged-out landing page) carry no
// participant; PostID / CommentID may be empty when the summon
// originates from a context-free endpoint.
type LoomSummon struct {
	ID              string          `json:"id" db:"id"`
	ParticipantID   *string         `json:"participant_id,omitempty" db:"participant_id"`
	PostID          *string         `json:"post_id,omitempty" db:"post_id"`
	CommentID       *string         `json:"comment_id,omitempty" db:"comment_id"`
	ReplyCommentID  *string         `json:"reply_comment_id,omitempty" db:"reply_comment_id"`
	Intent          LoomIntent      `json:"intent" db:"intent"`
	Prompt          string          `json:"prompt" db:"prompt"`
	Response        *string         `json:"response,omitempty" db:"response"`
	Model           *string         `json:"model,omitempty" db:"model"`
	InputTokens     *int            `json:"input_tokens,omitempty" db:"input_tokens"`
	OutputTokens    *int            `json:"output_tokens,omitempty" db:"output_tokens"`
	CostUSD         *float64        `json:"cost_usd,omitempty" db:"cost_usd"`
	Cached          bool            `json:"cached" db:"cached"`
	State           LoomSummonState `json:"state" db:"state"`
	ErrorCode       *string         `json:"error_code,omitempty" db:"error_code"`
	LatencyMs       *int            `json:"latency_ms,omitempty" db:"latency_ms"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty" db:"finished_at"`
}
