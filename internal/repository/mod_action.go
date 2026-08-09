package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ModActionRepo owns the per-community moderation log and the ban list.
// Actions are append-only; removals are reflected on the content itself
// via posts.removed_by_id / comments.removed_by_id, but every moderator
// intervention lands a row here for the mod log.
type ModActionRepo struct {
	pool *pgxpool.Pool
}

func NewModActionRepo(pool *pgxpool.Pool) *ModActionRepo {
	return &ModActionRepo{pool: pool}
}

// ModAction kinds — kept as plain strings so adding new actions doesn't
// need a schema change. Consumers should treat unknown kinds as opaque.
const (
	ModActionApprovePost  = "approve_post"
	ModActionRemovePost   = "remove_post"
	ModActionRestorePost  = "restore_post"
	ModActionRemoveComment = "remove_comment"
	ModActionRestoreComment = "restore_comment"
	ModActionBan          = "ban_user"
	ModActionUnban        = "unban_user"
)

type ModAction struct {
	ID          string    `json:"id"`
	CommunityID string    `json:"community_id"`
	ActorID     string    `json:"actor_id"`
	ActorName   string    `json:"actor_name,omitempty"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

type CommunityBan struct {
	CommunityID    string     `json:"community_id"`
	ParticipantID  string     `json:"participant_id"`
	ParticipantName string    `json:"participant_name,omitempty"`
	BannedByID     string     `json:"banned_by_id"`
	BannedByName   string     `json:"banned_by_name,omitempty"`
	Reason         string     `json:"reason"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Log appends an action. Never blocks or fails the caller if the log
// write fails — mod actions should succeed even if logging is down —
// but we surface the error so the handler can warn.
func (r *ModActionRepo) Log(ctx context.Context, communityID, actorID, action, targetType, targetID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO moderation_actions (community_id, actor_id, action, target_type, target_id, reason)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		communityID, actorID, action, targetType, targetID, reason)
	if err != nil {
		return fmt.Errorf("log mod action: %w", err)
	}
	return nil
}

// List returns recent actions for a community, newest first.
func (r *ModActionRepo) List(ctx context.Context, communityID string, limit int) ([]ModAction, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT ma.id, ma.community_id, ma.actor_id, p.display_name,
                ma.action, ma.target_type, ma.target_id, ma.reason, ma.created_at
         FROM moderation_actions ma
         JOIN participants p ON p.id = ma.actor_id
         WHERE ma.community_id = $1
         ORDER BY ma.created_at DESC
         LIMIT $2`, communityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list mod actions: %w", err)
	}
	defer rows.Close()

	out := []ModAction{}
	for rows.Next() {
		var a ModAction
		if err := rows.Scan(&a.ID, &a.CommunityID, &a.ActorID, &a.ActorName,
			&a.Action, &a.TargetType, &a.TargetID, &a.Reason, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mod action: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Ban inserts (or refreshes) a ban. Conflicting rows are overwritten —
// re-banning updates the reason/expiry and resets banned_by_id.
func (r *ModActionRepo) Ban(ctx context.Context, communityID, participantID, bannedByID, reason string, expiresAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO community_bans (community_id, participant_id, banned_by_id, reason, expires_at)
         VALUES ($1, $2, $3, $4, $5)
         ON CONFLICT (community_id, participant_id)
         DO UPDATE SET banned_by_id = EXCLUDED.banned_by_id,
                       reason       = EXCLUDED.reason,
                       expires_at   = EXCLUDED.expires_at,
                       created_at   = NOW()`,
		communityID, participantID, bannedByID, reason, expiresAt)
	if err != nil {
		return fmt.Errorf("ban user: %w", err)
	}
	return nil
}

// Unban removes the ban row if present. No error if the row didn't exist.
func (r *ModActionRepo) Unban(ctx context.Context, communityID, participantID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM community_bans WHERE community_id = $1 AND participant_id = $2`,
		communityID, participantID)
	if err != nil {
		return fmt.Errorf("unban user: %w", err)
	}
	return nil
}

// IsBanned is the check the post/comment create paths should call. An
// expired ban (expires_at < now) is treated as not-banned.
func (r *ModActionRepo) IsBanned(ctx context.Context, communityID, participantID string) (bool, error) {
	var banned bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
             SELECT 1 FROM community_bans
             WHERE community_id = $1 AND participant_id = $2
               AND (expires_at IS NULL OR expires_at > NOW())
         )`, communityID, participantID).Scan(&banned)
	if err != nil {
		return false, fmt.Errorf("check ban: %w", err)
	}
	return banned, nil
}

// ListBans returns currently-active bans for a community.
func (r *ModActionRepo) ListBans(ctx context.Context, communityID string) ([]CommunityBan, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT cb.community_id, cb.participant_id, p.display_name,
                cb.banned_by_id, bp.display_name,
                cb.reason, cb.expires_at, cb.created_at
         FROM community_bans cb
         JOIN participants p  ON p.id  = cb.participant_id
         JOIN participants bp ON bp.id = cb.banned_by_id
         WHERE cb.community_id = $1
           AND (cb.expires_at IS NULL OR cb.expires_at > NOW())
         ORDER BY cb.created_at DESC`, communityID)
	if err != nil {
		return nil, fmt.Errorf("list bans: %w", err)
	}
	defer rows.Close()

	out := []CommunityBan{}
	for rows.Next() {
		var b CommunityBan
		if err := rows.Scan(&b.CommunityID, &b.ParticipantID, &b.ParticipantName,
			&b.BannedByID, &b.BannedByName, &b.Reason, &b.ExpiresAt, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ban: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RemovePost marks a post as removed by a moderator. Sets deleted_at so
// the existing read paths hide it, but also records who and why so the
// distinction survives for the mod log UI and any future "removed by
// moderator" placeholder render.
func (r *ModActionRepo) RemovePost(ctx context.Context, postID, modID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE posts SET deleted_at = COALESCE(deleted_at, NOW()),
                          removed_by_id = $2,
                          removed_reason = $3
         WHERE id = $1`,
		postID, modID, reason)
	if err != nil {
		return fmt.Errorf("remove post: %w", err)
	}
	return nil
}

// RestorePost undoes a moderator removal. Only clears the fields we
// set; if the author had also self-deleted, the row stays hidden.
func (r *ModActionRepo) RestorePost(ctx context.Context, postID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE posts SET deleted_at = NULL,
                          removed_by_id = NULL,
                          removed_reason = ''
         WHERE id = $1 AND removed_by_id IS NOT NULL`,
		postID)
	if err != nil {
		return fmt.Errorf("restore post: %w", err)
	}
	return nil
}

// RemoveComment mirrors RemovePost for comments.
func (r *ModActionRepo) RemoveComment(ctx context.Context, commentID, modID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE comments SET deleted_at = COALESCE(deleted_at, NOW()),
                             removed_by_id = $2,
                             removed_reason = $3
         WHERE id = $1`,
		commentID, modID, reason)
	if err != nil {
		return fmt.Errorf("remove comment: %w", err)
	}
	return nil
}

func (r *ModActionRepo) RestoreComment(ctx context.Context, commentID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE comments SET deleted_at = NULL,
                             removed_by_id = NULL,
                             removed_reason = ''
         WHERE id = $1 AND removed_by_id IS NOT NULL`,
		commentID)
	if err != nil {
		return fmt.Errorf("restore comment: %w", err)
	}
	return nil
}

// PostCommunityID is a small helper the handlers use to check that a
// target post actually belongs to the community whose mod is acting.
func (r *ModActionRepo) PostCommunityID(ctx context.Context, postID string) (string, error) {
	var cid string
	err := r.pool.QueryRow(ctx, `SELECT community_id FROM posts WHERE id = $1`, postID).Scan(&cid)
	if err != nil {
		return "", err
	}
	return cid, nil
}

// CommentCommunityID resolves the community via the comment's parent post.
func (r *ModActionRepo) CommentCommunityID(ctx context.Context, commentID string) (string, error) {
	var cid string
	err := r.pool.QueryRow(ctx,
		`SELECT p.community_id FROM comments c JOIN posts p ON p.id = c.post_id WHERE c.id = $1`,
		commentID).Scan(&cid)
	if err != nil {
		return "", err
	}
	return cid, nil
}
