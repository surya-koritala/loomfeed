package activitypub

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RemoteTrustRepo persists per-instance trust state for remote
// fediverse actors. Rows are lazy — created on first observed
// interaction. See docs/FEDIVERSE_TRUST.md for the scoring model.
type RemoteTrustRepo struct {
	pool *pgxpool.Pool
}

func NewRemoteTrustRepo(pool *pgxpool.Pool) *RemoteTrustRepo {
	return &RemoteTrustRepo{pool: pool}
}

type RemoteTrust struct {
	RemoteActorURI string     `json:"remote_actor_uri"`
	LocalScore     float64    `json:"local_score"`
	AttestedScore  *float64   `json:"attested_score,omitempty"`
	AttestedIssuer *string    `json:"attested_issuer,omitempty"`
	AttestedAt     *time.Time `json:"attested_at,omitempty"`
	Interactions   int        `json:"interactions"`
	ReplyCount     int        `json:"reply_count"`
	ReplyVoteSum   int        `json:"reply_vote_sum"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
}

// RecordInteraction materializes the row on first sight and bumps
// the observable counters. `kind` is "follow", "reply", "reaction",
// etc. — opaque to this function; the distinction drives scoring
// in a future phase. For v1 we just count.
func (r *RemoteTrustRepo) RecordInteraction(ctx context.Context, remoteURI, kind string) error {
	_, err := r.pool.Exec(ctx, `
        INSERT INTO ap_remote_trust (remote_actor_uri, interactions, reply_count, last_seen_at)
        VALUES ($1, 1, CASE WHEN $2 = 'reply' THEN 1 ELSE 0 END, NOW())
        ON CONFLICT (remote_actor_uri) DO UPDATE SET
            interactions = ap_remote_trust.interactions + 1,
            reply_count  = ap_remote_trust.reply_count + (CASE WHEN $2 = 'reply' THEN 1 ELSE 0 END),
            last_seen_at = NOW()`,
		remoteURI, kind)
	if err != nil {
		return fmt.Errorf("record remote interaction: %w", err)
	}
	return r.recompute(ctx, remoteURI)
}

// StoreAttestation saves a verified advisory score from the remote's
// home instance. Caller MUST verify the signature first — this
// function only persists the row.
func (r *RemoteTrustRepo) StoreAttestation(ctx context.Context, remoteURI, issuer string, score float64, issuedAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
        INSERT INTO ap_remote_trust (remote_actor_uri, attested_score, attested_issuer, attested_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (remote_actor_uri) DO UPDATE SET
            attested_score = EXCLUDED.attested_score,
            attested_issuer = EXCLUDED.attested_issuer,
            attested_at = EXCLUDED.attested_at`,
		remoteURI, score, issuer, issuedAt)
	if err != nil {
		return fmt.Errorf("store attestation: %w", err)
	}
	return nil
}

// Get returns the current trust row for a remote actor, or nil if
// nothing has been recorded yet.
func (r *RemoteTrustRepo) Get(ctx context.Context, remoteURI string) (*RemoteTrust, error) {
	var t RemoteTrust
	err := r.pool.QueryRow(ctx, `
        SELECT remote_actor_uri, local_score, attested_score, attested_issuer,
               attested_at, interactions, reply_count, reply_vote_sum, last_seen_at
        FROM ap_remote_trust WHERE remote_actor_uri = $1`, remoteURI).
		Scan(&t.RemoteActorURI, &t.LocalScore, &t.AttestedScore, &t.AttestedIssuer,
			&t.AttestedAt, &t.Interactions, &t.ReplyCount, &t.ReplyVoteSum, &t.LastSeenAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// recompute runs the simple v1 scoring rule:
//   local_score = clamp(5 + 0.5 * reply_vote_sum, 0, 100)
// The cold-start floor is 5 and the cap is 100 to match local trust.
// Report-driven decay (per design doc) comes in a follow-up when we
// attach moderator actions to remote actors.
func (r *RemoteTrustRepo) recompute(ctx context.Context, remoteURI string) error {
	_, err := r.pool.Exec(ctx, `
        UPDATE ap_remote_trust
        SET local_score = LEAST(100, GREATEST(0, 5 + 0.5 * reply_vote_sum))
        WHERE remote_actor_uri = $1`, remoteURI)
	if err != nil {
		return fmt.Errorf("recompute remote trust: %w", err)
	}
	return nil
}
