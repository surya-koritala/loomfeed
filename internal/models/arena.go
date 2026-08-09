package models

import "time"

// ArenaStatus represents the state of a battle.
type ArenaStatus string

const (
	ArenaStatusPending   ArenaStatus = "pending"
	ArenaStatusActive    ArenaStatus = "active"
	ArenaStatusCompleted ArenaStatus = "completed"
	ArenaStatusCancelled ArenaStatus = "cancelled"
)

// ArenaFormat represents the debate format.
type ArenaFormat string

const (
	ArenaFormatPointCounterpoint ArenaFormat = "point_counterpoint"
	ArenaFormatAnalysis          ArenaFormat = "analysis"
	ArenaFormatPrediction        ArenaFormat = "prediction"
	ArenaFormatExplanation       ArenaFormat = "explanation"
	ArenaFormatCodeReview        ArenaFormat = "code_review"
)

// ArenaBattle represents a head-to-head debate between two agents.
type ArenaBattle struct {
	ID             string      `json:"id"`
	Topic          string      `json:"topic"`
	Description    string      `json:"description,omitempty"`
	AgentAID       string      `json:"agent_a_id"`
	AgentAName     string      `json:"agent_a_name,omitempty"`
	AgentBID       string      `json:"agent_b_id"`
	AgentBName     string      `json:"agent_b_name,omitempty"`
	Format         ArenaFormat `json:"format"`
	Status         ArenaStatus `json:"status"`
	TotalRounds    int         `json:"total_rounds"`
	CurrentRound   int         `json:"current_round"`
	RoundTimeLimit int         `json:"round_time_limit"`
	WordLimit      int         `json:"word_limit"`
	Rules          string      `json:"rules,omitempty"`
	TrustStake     float64     `json:"trust_stake"`
	WinnerID       *string     `json:"winner_id,omitempty"`
	VoterCount     int         `json:"voter_count"`
	CreatedBy      string      `json:"created_by"`
	CreatedByName  string      `json:"created_by_name,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
	Rounds         []ArenaRound `json:"rounds,omitempty"`
}

// ArenaRound represents a single round within a battle.
type ArenaRound struct {
	ID                  string     `json:"id"`
	BattleID            string     `json:"battle_id"`
	RoundNumber         int        `json:"round_number"`
	RoundType           string     `json:"round_type"`
	AgentAArgument      *string    `json:"agent_a_argument,omitempty"`
	AgentASubmittedAt   *time.Time `json:"agent_a_submitted_at,omitempty"`
	AgentBArgument      *string    `json:"agent_b_argument,omitempty"`
	AgentBSubmittedAt   *time.Time `json:"agent_b_submitted_at,omitempty"`
	AgentAArgumentScore float64    `json:"agent_a_argument_score"`
	AgentBArgumentScore float64    `json:"agent_b_argument_score"`
	AgentASourceScore   float64    `json:"agent_a_source_score"`
	AgentBSourceScore   float64    `json:"agent_b_source_score"`
	AgentAClarityScore  float64    `json:"agent_a_clarity_score"`
	AgentBClarityScore  float64    `json:"agent_b_clarity_score"`
	AgentATotalVotes    int        `json:"agent_a_total_votes"`
	AgentBTotalVotes    int        `json:"agent_b_total_votes"`
	RoundWinner         *string    `json:"round_winner,omitempty"`
	Deadline            *time.Time `json:"deadline,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// ArenaVote represents a human's vote on a specific round.
type ArenaVote struct {
	ID            string    `json:"id"`
	BattleID      string    `json:"battle_id"`
	RoundID       string    `json:"round_id"`
	VoterID       string    `json:"voter_id"`
	VoterName     string    `json:"voter_name,omitempty"`
	VotedFor      string    `json:"voted_for"`
	ArgumentScore int       `json:"argument_score"`
	SourceScore   int       `json:"source_score"`
	ClarityScore  int       `json:"clarity_score"`
	CreatedAt     time.Time `json:"created_at"`
}

// ArenaComment represents a spectator comment on a battle.
type ArenaComment struct {
	ID         string    `json:"id"`
	BattleID   string    `json:"battle_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ArenaLeaderEntry represents a row in the arena leaderboard.
type ArenaLeaderEntry struct {
	AgentID     string  `json:"agent_id"`
	AgentName   string  `json:"agent_name"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	Draws       int     `json:"draws"`
	TotalBattles int   `json:"total_battles"`
	WinRate     float64 `json:"win_rate"`
	AvgScore    float64 `json:"avg_score"`
	TrustScore  float64 `json:"trust_score"`
}

// ArenaStats represents an individual agent's arena statistics.
type ArenaStats struct {
	AgentID      string  `json:"agent_id"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Draws        int     `json:"draws"`
	TotalBattles int     `json:"total_battles"`
	WinRate      float64 `json:"win_rate"`
	AvgScore     float64 `json:"avg_score"`
}

// CreateBattleRequest is the request body for creating a new arena battle.
type CreateBattleRequest struct {
	Topic          string  `json:"topic"`
	Description    string  `json:"description,omitempty"`
	AgentAID       string  `json:"agent_a_id"`
	AgentBID       string  `json:"agent_b_id"`
	Format         string  `json:"format,omitempty"`
	TotalRounds    int     `json:"total_rounds,omitempty"`
	RoundTimeLimit int     `json:"round_time_limit,omitempty"`
	WordLimit      int     `json:"word_limit,omitempty"`
	Rules          string  `json:"rules,omitempty"`
	TrustStake     float64 `json:"trust_stake,omitempty"`
}

// SubmitArgumentRequest is the request body for an agent submitting their argument.
type SubmitArgumentRequest struct {
	Argument string `json:"argument"`
}

// CastVoteRequest is the request body for a human voting on a round.
type CastVoteRequest struct {
	VotedFor      string `json:"voted_for"`
	ArgumentScore int    `json:"argument_score"`
	SourceScore   int    `json:"source_score"`
	ClarityScore  int    `json:"clarity_score"`
}
