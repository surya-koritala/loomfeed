package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AccountRepo handles deletion-flow + data-export bookkeeping.
// Lives in its own file so the participant repo doesn't sprout a
// pile of GDPR helpers.
type AccountRepo struct {
	pool *pgxpool.Pool
}

func NewAccountRepo(pool *pgxpool.Pool) *AccountRepo {
	return &AccountRepo{pool: pool}
}

// SchedulePendingDeletion marks a participant for deletion. The
// hard-anonymization cron picks them up after 7 days. Idempotent:
// if already scheduled, the older timestamp is preserved (the user
// already started the clock).
func (r *AccountRepo) SchedulePendingDeletion(ctx context.Context, participantID string) (time.Time, error) {
	var ts time.Time
	err := r.pool.QueryRow(ctx, `
		UPDATE participants
		SET pending_deletion_at = COALESCE(pending_deletion_at, NOW())
		WHERE id = $1
		RETURNING pending_deletion_at`,
		participantID).Scan(&ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule deletion: %w", err)
	}
	return ts, nil
}

// CancelPendingDeletion clears pending_deletion_at. Called when the
// user logs in during the grace window or hits the explicit cancel
// button. No-op if no deletion was pending.
func (r *AccountRepo) CancelPendingDeletion(ctx context.Context, participantID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE participants
		SET pending_deletion_at = NULL
		WHERE id = $1 AND pending_deletion_at IS NOT NULL`,
		participantID)
	if err != nil {
		return fmt.Errorf("cancel deletion: %w", err)
	}
	return nil
}

// IsPending reports whether the participant has a pending deletion.
func (r *AccountRepo) IsPending(ctx context.Context, participantID string) (bool, *time.Time, error) {
	var ts *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT pending_deletion_at FROM participants WHERE id = $1`,
		participantID).Scan(&ts)
	if err != nil {
		return false, nil, fmt.Errorf("check pending deletion: %w", err)
	}
	return ts != nil, ts, nil
}

// ListReadyForAnonymization returns IDs of participants whose
// 7-day grace has elapsed. The cron worker calls this once per
// run and anonymizes each ID it gets back.
func (r *AccountRepo) ListReadyForAnonymization(ctx context.Context, graceDays int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM participants
		WHERE pending_deletion_at IS NOT NULL
		  AND pending_deletion_at < NOW() - $1::int * INTERVAL '1 day'`,
		graceDays)
	if err != nil {
		return nil, fmt.Errorf("list ready for anonymization: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Anonymize is the GDPR Article 17 hard-delete: replace personal
// data with a sentinel "[deleted]" marker but keep the row so
// post/comment FKs hold and threads stay coherent.
//
// What gets blanked:
//   - participants.display_name → "[deleted]"
//   - participants.avatar_url   → ''
//   - participants.bio          → ''
//   - participants.pending_deletion_at → NULL (resolved)
//   - human_users.email         → 'deleted-{id}@deleted.local'
//   - human_users.password_hash → '!' (uncrackable, can't log in)
//   - human_users.oauth_provider → ''
//   - api_keys (for agents)     → all rows revoked (is_active = FALSE)
//
// What we preserve:
//   - posts.body / comments.body — others may have replied; keeping
//     the content with an anonymous author is the standard interpretation
//     of "right to erasure" balanced against others' freedom of expression.
//   - reputation_score / trust_score — historical, not personal data.
//
// Wrapped in a transaction so partial anonymization can't happen.
func (r *AccountRepo) Anonymize(ctx context.Context, participantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE participants
		SET display_name = '[deleted]',
		    avatar_url = NULL,
		    bio = NULL,
		    pending_deletion_at = NULL
		WHERE id = $1`, participantID); err != nil {
		return fmt.Errorf("anonymize participant: %w", err)
	}

	// Human-only fields. UPDATE is a no-op for agents (no row).
	if _, err := tx.Exec(ctx, `
		UPDATE human_users
		SET email = 'deleted-' || participant_id::text || '@deleted.local',
		    password_hash = '!',
		    oauth_provider = NULL,
		    failed_login_count = 0,
		    locked_until = NULL
		WHERE participant_id = $1`, participantID); err != nil {
		return fmt.Errorf("anonymize human user: %w", err)
	}

	// Revoke any API keys the participant owned (only relevant for
	// agents, but harmless on humans). Doesn't drop rows so audit
	// history of "this key existed" survives.
	if _, err := tx.Exec(ctx, `
		UPDATE api_keys
		SET is_active = FALSE,
		    revoked_at = COALESCE(revoked_at, NOW()),
		    revoke_reason = COALESCE(revoke_reason, 'account_deleted')
		WHERE agent_id = $1`, participantID); err != nil {
		return fmt.Errorf("revoke api keys: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit anonymize: %w", err)
	}
	return nil
}

// GraduatedAt returns when this participant became "trusted enough"
// to skip the new-account quarantine. NULL = not graduated yet
// (post creation will be quarantined). Only humans are quarantined,
// so this is mostly a no-op for agents — but the column exists on
// every participant so the query is uniform.
func (r *AccountRepo) GraduatedAt(ctx context.Context, participantID string) (*time.Time, error) {
	var ts *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT graduated_at FROM participants WHERE id = $1`,
		participantID).Scan(&ts)
	if err != nil {
		return nil, fmt.Errorf("read graduated_at: %w", err)
	}
	return ts, nil
}

// MarkGraduated stamps graduated_at so future posts skip the
// quarantine check. Idempotent — re-graduating preserves the
// original timestamp.
func (r *AccountRepo) MarkGraduated(ctx context.Context, participantID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE participants
		SET graduated_at = COALESCE(graduated_at, NOW())
		WHERE id = $1`,
		participantID)
	if err != nil {
		return fmt.Errorf("mark graduated: %w", err)
	}
	return nil
}

// LogExport records a data-export request. Today the handler streams
// the export synchronously; the row is purely for audit + future
// async migration.
func (r *AccountRepo) LogExport(ctx context.Context, participantID string, rowCount int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO data_exports (participant_id, status, row_count, delivered_at)
		VALUES ($1, 'ready', $2, NOW())`,
		participantID, rowCount)
	if err != nil {
		return fmt.Errorf("log export: %w", err)
	}
	return nil
}
