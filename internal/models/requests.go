package models

import "time"

// === Auth ===

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	// Optional invite code. If valid, links the new account to the
	// inviter and records a reputation event. Unknown codes are
	// silently dropped — we don't want a mistyped code to fail signup.
	InviteCode string `json:"invite_code,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token        string       `json:"token,omitempty"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	ExpiresIn    int          `json:"expires_in"`
	Participant  *Participant `json:"participant"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type RefreshToken struct {
	ID            string     `json:"id" db:"id"`
	ParticipantID string     `json:"participant_id" db:"participant_id"`
	TokenHash     string     `json:"-" db:"token_hash"`
	ExpiresAt     time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
}

// === Agent ===

type RegisterAgentRequest struct {
	DisplayName   string       `json:"display_name"`
	Bio           string       `json:"bio,omitempty"`
	ModelProvider string       `json:"model_provider"`
	ModelName     string       `json:"model_name"`
	ModelVersion  string       `json:"model_version,omitempty"`
	Capabilities  []string     `json:"capabilities,omitempty"`
	ProtocolType  ProtocolType `json:"protocol_type"`
	AgentURL      string       `json:"agent_url,omitempty"`
}

type RegisterAgentResponse struct {
	Agent  *AgentIdentity `json:"agent"`
	APIKey string         `json:"api_key"` // only shown once at creation
}

// === Community ===

type CreateCommunityRequest struct {
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description,omitempty"`
	Rules       string      `json:"rules,omitempty"`
	AgentPolicy AgentPolicy `json:"agent_policy,omitempty"`
	Category    string      `json:"category,omitempty"`
}

// === Post ===

type CreatePostRequest struct {
	CommunityID     string         `json:"community_id"`
	Title           string         `json:"title"`
	Body            string         `json:"body"`
	URL             string         `json:"url,omitempty"`
	PostType        string         `json:"post_type,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Sources         []string       `json:"sources,omitempty"`
	ConfidenceScore *float64       `json:"confidence_score,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	// QuotedPostID — Phase 1.2 quote-post pattern. When set, the
	// new post's body sits below an inset citation card pointing
	// at the quoted post.
	QuotedPostID *string `json:"quoted_post_id,omitempty"`
}

// CreatePostResponse is the stable 201 representation for post creation.
// Provenance is present when the request supplied sources.
type CreatePostResponse struct {
	Post
	Provenance *Provenance `json:"provenance,omitempty"`
}

// === Comment ===

type CreateCommentRequest struct {
	PostID          string   `json:"post_id"`
	ParentCommentID *string  `json:"parent_comment_id,omitempty"`
	Body            string   `json:"body"`
	Sources         []string `json:"sources,omitempty"`
	ConfidenceScore *float64 `json:"confidence_score,omitempty"`
	IsAnswer        bool     `json:"is_answer"`
	// "main" (default) or "talk" — Wikipedia-style meta discussion.
	ThreadType string `json:"thread_type,omitempty"`
}

// === Vote ===

type VoteRequest struct {
	TargetID   string `json:"target_id"`
	TargetType string `json:"target_type"` // "post" or "comment"
	Direction  string `json:"direction"`   // "up" or "down"
}

// === Feed ===

type FeedQuery struct {
	CommunitySlug string
	Sort          string // "hot", "new", "top", "rising"
	Type          string // filter by post_type
	Limit         int
	Offset        int
}

// === Generic ===

type PostWithAuthor struct {
	Post
	Author     Participant `json:"author"`
	Community  *Community  `json:"community,omitempty"`
	Provenance *Provenance `json:"provenance,omitempty"`
	// QuotedPost — Phase 1.2. When this post quotes another, the
	// detail endpoint embeds the quoted post here so the frontend
	// can render an inset citation card without a second round-trip.
	// Feed responses leave this nil and the frontend renders a
	// lightweight "QUOTED" pill from quoted_post_id alone.
	QuotedPost       *PostWithAuthor `json:"quoted_post,omitempty"`
	UserVote         *string         `json:"user_vote"`
	UserBookmarked   bool            `json:"user_bookmarked"`
	AuthorScore      *float64        `json:"author_score"`
	AuthorTier       string          `json:"author_tier"`
	QualityScore     *float64        `json:"quality_score"`
	VerifiedSources  int             `json:"verified_sources"`
	TotalSources     int             `json:"total_sources"`
	EpistemicStatus  *string         `json:"epistemic_status"`
	AuthorFlairLabel string          `json:"author_flair_label,omitempty"`
	AuthorFlairColor string          `json:"author_flair_color,omitempty"`
	// ViewerFollowing: the authenticated requester follows this post's
	// author. Populated only on authed feed/detail responses; powers the
	// in-context Subscribe CTA. Omitted (false) for anonymous requests.
	ViewerFollowing bool `json:"viewer_following"`
}

// SearchResult wraps a PostWithAuthor with a relevance score from hybrid search.
type SearchResult struct {
	PostWithAuthor
	RelevanceScore float64 `json:"relevance_score"`
}

// SearchResponse is the response envelope for search results.
type SearchResponse struct {
	Data        []SearchResult `json:"data"`
	Total       int            `json:"total"`
	Query       string         `json:"query"`
	Mode        string         `json:"mode"`
	Limit       int            `json:"limit"`
	Offset      int            `json:"offset"`
	HasMore     bool           `json:"has_more"`
	NextCursor  string         `json:"next_cursor,omitempty"`
	Community   string         `json:"community,omitempty"`
	AuthorType  string         `json:"author_type,omitempty"`
	PostType    string         `json:"post_type,omitempty"`
	Period      string         `json:"period,omitempty"`
	RetrievedAt time.Time      `json:"retrieved_at"`
}

type CommentWithAuthor struct {
	Comment
	Author         Participant `json:"author"`
	Provenance     *Provenance `json:"provenance,omitempty"`
	UserVote       *string     `json:"user_vote"`
	UserBookmarked bool        `json:"user_bookmarked"`
}

type PaginatedResponse struct {
	Data        any       `json:"data"`
	Total       int       `json:"total"`
	Limit       int       `json:"limit"`
	Offset      int       `json:"offset"`
	HasMore     bool      `json:"has_more"`
	NextCursor  string    `json:"next_cursor,omitempty"`
	RetrievedAt time.Time `json:"retrieved_at"`
}
