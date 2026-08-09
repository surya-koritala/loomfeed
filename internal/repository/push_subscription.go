package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PushSubscription struct {
	ID            string     `json:"id"`
	ParticipantID string     `json:"participant_id"`
	Endpoint      string     `json:"endpoint"`
	P256dhKey     string     `json:"p256dh_key"`
	AuthKey       string     `json:"auth_key"`
	UserAgent     string     `json:"user_agent,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

type PushSubscriptionRepo struct {
	pool *pgxpool.Pool
}

func NewPushSubscriptionRepo(pool *pgxpool.Pool) *PushSubscriptionRepo {
	return &PushSubscriptionRepo{pool: pool}
}

// Upsert inserts a subscription or refreshes the keys if the endpoint
// already exists (same device subscribing twice — rotate the keys).
func (r *PushSubscriptionRepo) Upsert(ctx context.Context, participantID, endpoint, p256dh, auth, userAgent string) error {
	_, err := r.pool.Exec(ctx, `
        INSERT INTO push_subscriptions (participant_id, endpoint, p256dh_key, auth_key, user_agent)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (endpoint) DO UPDATE SET
            participant_id = EXCLUDED.participant_id,
            p256dh_key = EXCLUDED.p256dh_key,
            auth_key = EXCLUDED.auth_key,
            user_agent = EXCLUDED.user_agent`,
		participantID, endpoint, p256dh, auth, userAgent)
	if err != nil {
		return fmt.Errorf("upsert push subscription: %w", err)
	}
	return nil
}

// DeleteByEndpoint removes one subscription, used both by /unsubscribe
// and by the send pipeline when the endpoint returns 410/404.
func (r *PushSubscriptionRepo) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	return err
}

// ListByParticipant returns every active subscription for a user.
func (r *PushSubscriptionRepo) ListByParticipant(ctx context.Context, participantID string) ([]PushSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, participant_id, endpoint, p256dh_key, auth_key, COALESCE(user_agent, ''), created_at, last_used_at
         FROM push_subscriptions WHERE participant_id = $1`, participantID)
	if err != nil {
		return nil, fmt.Errorf("list push subs: %w", err)
	}
	defer rows.Close()
	out := []PushSubscription{}
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.ID, &s.ParticipantID, &s.Endpoint, &s.P256dhKey, &s.AuthKey, &s.UserAgent, &s.CreatedAt, &s.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan push sub: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// MarkUsed bumps last_used_at — useful for pruning stale subs later.
func (r *PushSubscriptionRepo) MarkUsed(ctx context.Context, endpoint string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE push_subscriptions SET last_used_at = NOW() WHERE endpoint = $1`, endpoint)
	return err
}
