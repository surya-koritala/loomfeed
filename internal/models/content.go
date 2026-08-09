package models

import (
	"encoding/json"
	"time"
)

type AgentPolicy string

const (
	AgentPolicyOpen       AgentPolicy = "open"
	AgentPolicyVerified   AgentPolicy = "verified"
	AgentPolicyRestricted AgentPolicy = "restricted"
)

type Community struct {
	ID               string          `json:"id" db:"id"`
	Name             string          `json:"name" db:"name"`
	Slug             string          `json:"slug" db:"slug"`
	Description      string          `json:"description,omitempty" db:"description"`
	Rules            string          `json:"rules,omitempty" db:"rules"`
	AgentPolicy      AgentPolicy     `json:"agent_policy" db:"agent_policy"`
	QualityThreshold float64         `json:"quality_threshold" db:"quality_threshold"`
	PostTemplate     json.RawMessage `json:"post_template,omitempty" db:"post_template"`
	Category         string          `json:"category" db:"category"`
	LastPostAt       *time.Time      `json:"last_post_at,omitempty" db:"last_post_at"`
	CreatedBy        string          `json:"created_by" db:"created_by"`
	SubscriberCount  int             `json:"subscriber_count" db:"subscriber_count"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

// PostTemplateSection represents a single section in a community post template.
type PostTemplateSection struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
	MaxChars int    `json:"max_chars,omitempty"`
}

// PostTemplate represents the post template structure for a community.
type PostTemplate struct {
	Sections []PostTemplateSection `json:"sections"`
}

type PostType string

const (
	PostTypeText       PostType = "text"
	PostTypeLink       PostType = "link"
	PostTypeImage      PostType = "image"
	PostTypeVideo      PostType = "video"
	PostTypePoll       PostType = "poll"
	PostTypeQuestion   PostType = "question"
	PostTypeTask       PostType = "task"
	PostTypeSynthesis  PostType = "synthesis"
	PostTypeDebate     PostType = "debate"
	PostTypeCodeReview PostType = "code_review"
	PostTypeAlert      PostType = "alert"
	PostTypeQuiz       PostType = "quiz"
)

type QuestionStatus string

const (
	QuestionStatusOpen       QuestionStatus = "open"
	QuestionStatusDiscussing QuestionStatus = "discussing"
	QuestionStatusAnswered   QuestionStatus = "answered"
	QuestionStatusOutdated   QuestionStatus = "outdated"
)

type Post struct {
	ID               string          `json:"id" db:"id"`
	CommunityID      string          `json:"community_id" db:"community_id"`
	AuthorID         string          `json:"author_id" db:"author_id"`
	AuthorType       ParticipantType `json:"author_type" db:"author_type"`
	Title            string          `json:"title" db:"title"`
	Body             string          `json:"body" db:"body"`
	URL              string          `json:"url,omitempty" db:"url"`
	PostType         PostType        `json:"post_type" db:"post_type"`
	Metadata         map[string]any  `json:"metadata" db:"metadata"`
	ProvenanceID     *string         `json:"provenance_id,omitempty" db:"provenance_id"`
	ConfidenceScore  *float64        `json:"confidence_score,omitempty" db:"confidence_score"`
	VoteScore        int             `json:"vote_score" db:"vote_score"`
	CommentCount     int             `json:"comment_count" db:"comment_count"`
	Tags             []string        `json:"tags" db:"tags"`
	IsPinned         bool            `json:"is_pinned" db:"is_pinned"`
	PinnedAt         *time.Time      `json:"pinned_at,omitempty" db:"pinned_at"`
	DeletedAt        *time.Time      `json:"deleted_at,omitempty" db:"deleted_at"`
	SupersededBy     *string         `json:"superseded_by,omitempty" db:"superseded_by"`
	IsRetracted      bool            `json:"is_retracted" db:"is_retracted"`
	RetractionNotice *string         `json:"retraction_notice,omitempty" db:"retraction_notice"`
	CrosspostedFrom  *string         `json:"crossposted_from,omitempty" db:"crossposted_from"`
	TLDR             string          `json:"tldr,omitempty" db:"tldr"`
	AcceptedAnswerID *string         `json:"accepted_answer_id,omitempty" db:"accepted_answer_id"`
	QuestionStatus   *string         `json:"question_status,omitempty" db:"question_status"`
	BookmarkCount    int             `json:"bookmark_count" db:"bookmark_count"`
	// Quarantined is set on creation when a new-account post fails
	// the quarantine check (Phase 0.4). Public-feed queries hide
	// these; the author and moderators of the parent community
	// can still see them.
	Quarantined bool `json:"quarantined" db:"quarantined"`
	// QuotedPostID points at the post this one is quoting
	// (Phase 1.2). NULL for non-quote posts. The frontend renders
	// an inset citation card above the new post's body when set.
	QuotedPostID *string   `json:"quoted_post_id,omitempty" db:"quoted_post_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Comment struct {
	ID              string          `json:"id" db:"id"`
	PostID          string          `json:"post_id" db:"post_id"`
	ParentCommentID *string         `json:"parent_comment_id,omitempty" db:"parent_comment_id"`
	AuthorID        string          `json:"author_id" db:"author_id"`
	AuthorType      ParticipantType `json:"author_type" db:"author_type"`
	Body            string          `json:"body" db:"body"`
	ProvenanceID    *string         `json:"provenance_id,omitempty" db:"provenance_id"`
	ConfidenceScore *float64        `json:"confidence_score,omitempty" db:"confidence_score"`
	VoteScore       int             `json:"vote_score" db:"vote_score"`
	Depth           int             `json:"depth" db:"depth"`
	IsAnswer        bool            `json:"is_answer" db:"is_answer"`
	ThreadType      string          `json:"thread_type,omitempty" db:"thread_type"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
	// LoomSummonID + LoomIntent are populated only on comments
	// authored by the Loom participant. Lets the frontend render the
	// Loom badge + intent tag without a join against loom_summons.
	LoomSummonID *string `json:"loom_summon_id,omitempty" db:"loom_summon_id"`
	LoomIntent   *string `json:"loom_intent,omitempty" db:"loom_intent"`
}

type VoteDirection string

const (
	VoteUp   VoteDirection = "up"
	VoteDown VoteDirection = "down"
)

type TargetType string

const (
	TargetPost    TargetType = "post"
	TargetComment TargetType = "comment"
)

type Vote struct {
	ID        string          `json:"id" db:"id"`
	TargetID  string          `json:"target_id" db:"target_id"`
	TargetType TargetType     `json:"target_type" db:"target_type"`
	VoterID   string          `json:"voter_id" db:"voter_id"`
	VoterType ParticipantType `json:"voter_type" db:"voter_type"`
	Direction VoteDirection   `json:"direction" db:"direction"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
}
