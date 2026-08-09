package models

import "time"

type GenerationMethod string

const (
	MethodOriginal    GenerationMethod = "original"
	MethodSynthesis   GenerationMethod = "synthesis"
	MethodSummary     GenerationMethod = "summary"
	MethodTranslation GenerationMethod = "translation"
)

type Provenance struct {
	ID               string           `json:"id" db:"id"`
	ContentID        string           `json:"content_id" db:"content_id"`
	ContentType      TargetType       `json:"content_type" db:"content_type"`
	AuthorID         string           `json:"author_id" db:"author_id"`
	Sources          []string         `json:"sources" db:"sources"`
	ModelUsed        string           `json:"model_used,omitempty" db:"model_used"`
	ModelVersion     string           `json:"model_version,omitempty" db:"model_version"`
	PromptHash       string           `json:"prompt_hash,omitempty" db:"prompt_hash"`
	ConfidenceScore  float64          `json:"confidence_score" db:"confidence_score"`
	GenerationMethod GenerationMethod `json:"generation_method" db:"generation_method"`
	CreatedAt        time.Time        `json:"created_at" db:"created_at"`
}

type CitationType string

const (
	CitationSupports    CitationType = "supports"
	CitationContradicts CitationType = "contradicts"
	CitationExtends     CitationType = "extends"
	CitationQuotes      CitationType = "quotes"
)

type CitationEdge struct {
	SourceContentID string       `json:"source_content_id" db:"source_content_id"`
	CitedContentID  string       `json:"cited_content_id" db:"cited_content_id"`
	CitationType    CitationType `json:"citation_type" db:"citation_type"`
	ContextSnippet  string       `json:"context_snippet,omitempty" db:"context_snippet"`
}

type ReputationEventType string

const (
	EventUpvoteReceived  ReputationEventType = "upvote_received"
	EventContentVerified ReputationEventType = "content_verified"
	EventFlagUpheld      ReputationEventType = "flag_upheld"
	EventAgentEndorsed   ReputationEventType = "agent_endorsed"
)

type ReputationEvent struct {
	ID            string              `json:"id" db:"id"`
	ParticipantID string              `json:"participant_id" db:"participant_id"`
	EventType     ReputationEventType `json:"event_type" db:"event_type"`
	ScoreDelta    float64             `json:"score_delta" db:"score_delta"`
	CreatedAt     time.Time           `json:"created_at" db:"created_at"`
}

type QualityGate struct {
	ID                     string  `json:"id" db:"id"`
	CommunityID            string  `json:"community_id" db:"community_id"`
	MinTrustScore          float64 `json:"min_trust_score" db:"min_trust_score"`
	MinConfidenceScore     float64 `json:"min_confidence_score" db:"min_confidence_score"`
	RequireProvenance      bool    `json:"require_provenance" db:"require_provenance"`
	RequireHumanVerify     bool    `json:"require_human_verification" db:"require_human_verification"`
	MaxAgentPostsPerHour   int     `json:"max_agent_posts_per_hour" db:"max_agent_posts_per_hour"`
}
