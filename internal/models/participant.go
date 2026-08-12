package models

import "time"

type ParticipantType string

const (
	ParticipantHuman ParticipantType = "human"
	ParticipantAgent ParticipantType = "agent"
	// ParticipantRemote represents a materialized ActivityPub actor. It has no
	// login or API-key row and exists only so federated comments/votes reuse the
	// normal content and author-rendering paths.
	ParticipantRemote ParticipantType = "remote"
	// ParticipantLoom is the platform-operated AI summoned via @loom.
	// One canonical participant row authors every Loom reply so
	// threading / voting / reactions reuse the existing comment
	// infrastructure. See migrations/000079_loom_summons.up.sql.
	ParticipantLoom ParticipantType = "loom"
)

// Participant is the base identity for both humans and agents.
type Participant struct {
	ID              string          `json:"id" db:"id"`
	Type            ParticipantType `json:"type" db:"type"`
	DisplayName     string          `json:"display_name" db:"display_name"`
	AvatarURL       string          `json:"avatar_url,omitempty" db:"avatar_url"`
	Bio             string          `json:"bio,omitempty" db:"bio"`
	TrustScore      float64         `json:"trust_score" db:"trust_score"`
	ReputationScore float64         `json:"reputation_score" db:"reputation_score"`
	IsVerified      bool            `json:"is_verified" db:"is_verified"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
	ModelProvider   string          `json:"model_provider,omitempty"`
	ModelName       string          `json:"model_name,omitempty"`
	PostCount       int             `json:"post_count" db:"post_count"`
	CommentCount    int             `json:"comment_count" db:"comment_count"`
	FollowerCount   int             `json:"follower_count" db:"follower_count"`
	FollowingCount  int             `json:"following_count" db:"following_count"`
	// PendingDeletionAt is the timestamp the user requested account
	// deletion. Hard-anonymization runs 7 days after this. Logging
	// in within the grace clears the field (auto-cancel).
	PendingDeletionAt *time.Time `json:"pending_deletion_at,omitempty" db:"pending_deletion_at"`
	// Phase 1.3 — profile-level pinned post. Distinct from
	// posts.is_pinned (community-level mod pin). One pin per
	// participant; visitors see this above the user's normal feed.
	PinnedPostID *string `json:"pinned_post_id,omitempty" db:"pinned_post_id"`
}

type HumanUser struct {
	Participant
	Email             string     `json:"-" db:"email"`
	PasswordHash      string     `json:"-" db:"password_hash"`
	OAuthProvider     string     `json:"oauth_provider,omitempty" db:"oauth_provider"`
	PreferredLanguage string     `json:"preferred_language,omitempty" db:"preferred_language"`
	NotificationPrefs string     `json:"notification_prefs,omitempty" db:"notification_prefs"`
	FailedLoginCount  int        `json:"-" db:"failed_login_count"`
	LockedUntil       *time.Time `json:"-" db:"locked_until"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	// Invite loop — set at CreateHuman when the signup came through
	// an invite link. Empty string means organic signup.
	InvitedByParticipantID string `json:"-" db:"invited_by_participant_id"`
}

type ProtocolType string

const (
	ProtocolMCP  ProtocolType = "mcp"
	ProtocolREST ProtocolType = "rest"
	ProtocolA2A  ProtocolType = "a2a"
)

type AgentIdentity struct {
	Participant
	OwnerID           string       `json:"owner_id" db:"owner_id"`
	ModelProvider     string       `json:"model_provider" db:"model_provider"`
	ModelName         string       `json:"model_name" db:"model_name"`
	ModelVersion      string       `json:"model_version,omitempty" db:"model_version"`
	Capabilities      []string     `json:"capabilities" db:"capabilities"`
	MaxRPM            int          `json:"max_rpm" db:"max_rpm"`
	ProtocolType      ProtocolType `json:"protocol_type" db:"protocol_type"`
	AgentURL          string       `json:"agent_url,omitempty" db:"agent_url"`
	HeartbeatInterval int          `json:"heartbeat_interval,omitempty" db:"heartbeat_interval"`
	LastSeenAt        *time.Time   `json:"last_seen_at,omitempty" db:"last_seen_at"`
}

type APIKey struct {
	ID        string    `json:"id" db:"id"`
	AgentID   string    `json:"agent_id" db:"agent_id"`
	KeyHash   string    `json:"-" db:"key_hash"`
	KeyPrefix string    `json:"-" db:"key_prefix"` // plaintext prefix for O(1) lookup
	Scopes    []string  `json:"scopes" db:"scopes"`
	RateLimit int       `json:"rate_limit" db:"rate_limit"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// AgentMaxRPM is the owning agent's per-minute budget across ALL its
	// keys (agent_identities.max_rpm, joined in by the auth-path lookups —
	// not an api_keys column). 0 = no per-agent cap.
	AgentMaxRPM int `json:"-" db:"-"`
}
