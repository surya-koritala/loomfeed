package activitypub

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FollowersRepo persists remote fediverse followers. Stored in the
// ap_followers table introduced by migration 000053.
type FollowersRepo struct {
	pool *pgxpool.Pool
}

func NewFollowersRepo(pool *pgxpool.Pool) *FollowersRepo {
	return &FollowersRepo{pool: pool}
}

type Follower struct {
	ID             string    `json:"id"`
	LocalActorID   string    `json:"local_actor_id"`
	RemoteActorURI string    `json:"remote_actor_uri"`
	InboxURI       string    `json:"inbox_uri"`
	SharedInboxURI string    `json:"shared_inbox_uri,omitempty"`
	AcceptedAt     time.Time `json:"accepted_at"`
}

// Upsert inserts a new follower row; if the same remote already
// follows this local actor, refreshes their inbox URIs.
func (r *FollowersRepo) Upsert(ctx context.Context, localID, remoteURI, inbox, sharedInbox string) error {
	_, err := r.pool.Exec(ctx, `
        INSERT INTO ap_followers (local_actor_id, remote_actor_uri, inbox_uri, shared_inbox_uri)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (local_actor_id, remote_actor_uri) DO UPDATE SET
            inbox_uri = EXCLUDED.inbox_uri,
            shared_inbox_uri = EXCLUDED.shared_inbox_uri,
            accepted_at = NOW()`,
		localID, remoteURI, inbox, sharedInbox)
	if err != nil {
		return fmt.Errorf("upsert follower: %w", err)
	}
	return nil
}

// Remove deletes a follower relationship. Called on incoming Undo Follow.
func (r *FollowersRepo) Remove(ctx context.Context, localID, remoteURI string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM ap_followers WHERE local_actor_id = $1 AND remote_actor_uri = $2`,
		localID, remoteURI)
	return err
}

// ListForDelivery returns the unique set of inbox URIs we should POST
// to when fanning out an activity from localID. Uses shared_inbox when
// available to cut request count; falls back to the per-actor inbox.
func (r *FollowersRepo) ListForDelivery(ctx context.Context, localID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT DISTINCT COALESCE(NULLIF(shared_inbox_uri, ''), inbox_uri)
        FROM ap_followers
        WHERE local_actor_id = $1`, localID)
	if err != nil {
		return nil, fmt.Errorf("list delivery targets: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		out = append(out, uri)
	}
	return out, rows.Err()
}

// ListFollowers returns (uri-only) the followers of localID, paginated.
// Used by GET /users/{handle}/followers collection.
func (r *FollowersRepo) ListFollowers(ctx context.Context, localID string, limit, offset int) ([]string, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ap_followers WHERE local_actor_id = $1`, localID).Scan(&total)

	rows, err := r.pool.Query(ctx,
		`SELECT remote_actor_uri FROM ap_followers
         WHERE local_actor_id = $1
         ORDER BY accepted_at DESC
         LIMIT $2 OFFSET $3`, localID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list followers: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return nil, 0, err
		}
		out = append(out, uri)
	}
	return out, total, rows.Err()
}
