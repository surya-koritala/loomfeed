package activitypub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboundFollowStatus string

const (
	OutboundFollowPending  OutboundFollowStatus = "pending"
	OutboundFollowAccepted OutboundFollowStatus = "accepted"
)

type OutboundFollow struct {
	ID             string               `json:"id"`
	LocalActorID   string               `json:"local_actor_id"`
	RemoteActorURI string               `json:"remote_actor_uri"`
	RemoteInboxURI string               `json:"remote_inbox_uri"`
	ActivityID     string               `json:"activity_id"`
	Status         OutboundFollowStatus `json:"status"`
	LastDeliveryAt *time.Time           `json:"last_delivery_at,omitempty"`
	LastError      *string              `json:"last_error,omitempty"`
	AcceptedAt     *time.Time           `json:"accepted_at,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type OutboundFollowRepo struct {
	pool *pgxpool.Pool
}

func NewOutboundFollowRepo(pool *pgxpool.Pool) *OutboundFollowRepo {
	return &OutboundFollowRepo{pool: pool}
}

// Ensure creates a stable pending Follow activity. Repeated requests refresh
// the cached inbox but retain the original activity ID so remote delivery is
// idempotent and a later Accept can always be correlated.
func (r *OutboundFollowRepo) Ensure(ctx context.Context, localActorID, remoteActorURI, remoteInboxURI, activityID string) (*OutboundFollow, bool, error) {
	var follow OutboundFollow
	var created bool
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ap_outbound_follows
			(local_actor_id, remote_actor_uri, remote_inbox_uri, activity_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (local_actor_id, remote_actor_uri) DO UPDATE SET
			remote_inbox_uri = EXCLUDED.remote_inbox_uri,
			updated_at = NOW()
		RETURNING id, local_actor_id, remote_actor_uri, remote_inbox_uri,
			activity_id, status, last_delivery_at, last_error, accepted_at,
			created_at, updated_at, (xmax = 0)`,
		localActorID, remoteActorURI, remoteInboxURI, activityID,
	).Scan(
		&follow.ID, &follow.LocalActorID, &follow.RemoteActorURI, &follow.RemoteInboxURI,
		&follow.ActivityID, &follow.Status, &follow.LastDeliveryAt, &follow.LastError,
		&follow.AcceptedAt, &follow.CreatedAt, &follow.UpdatedAt, &created,
	)
	if err != nil {
		return nil, false, fmt.Errorf("ensure outbound follow: %w", err)
	}
	return &follow, created, nil
}

func (r *OutboundFollowRepo) RecordDelivery(ctx context.Context, id string, deliveryErr error) error {
	var lastError *string
	if deliveryErr != nil {
		message := deliveryErr.Error()
		if len(message) > 1000 {
			message = message[:1000]
		}
		lastError = &message
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE ap_outbound_follows
		SET last_delivery_at = NOW(), last_error = $2, updated_at = NOW()
		WHERE id = $1`, id, lastError)
	if err != nil {
		return fmt.Errorf("record outbound Follow delivery: %w", err)
	}
	return nil
}

// Accept transitions only the exact local actor, remote actor, and activity
// tuple. A signed Accept from one remote can never claim another remote's
// pending Follow.
func (r *OutboundFollowRepo) Accept(ctx context.Context, localActorID, remoteActorURI, activityID string) (bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		UPDATE ap_outbound_follows
		SET status = 'accepted', accepted_at = COALESCE(accepted_at, NOW()),
			last_error = NULL, updated_at = NOW()
		WHERE local_actor_id = $1 AND remote_actor_uri = $2 AND activity_id = $3
		RETURNING id`, localActorID, remoteActorURI, activityID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("accept outbound follow: %w", err)
	}
	return true, nil
}

func (r *OutboundFollowRepo) GetOwned(ctx context.Context, localActorID, id string) (*OutboundFollow, error) {
	return scanOutboundFollow(r.pool.QueryRow(ctx, `
		SELECT id, local_actor_id, remote_actor_uri, remote_inbox_uri,
			activity_id, status, last_delivery_at, last_error, accepted_at,
			created_at, updated_at
		FROM ap_outbound_follows
		WHERE id = $1 AND local_actor_id = $2`, id, localActorID))
}

func (r *OutboundFollowRepo) ListByLocal(ctx context.Context, localActorID string) ([]OutboundFollow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, local_actor_id, remote_actor_uri, remote_inbox_uri,
			activity_id, status, last_delivery_at, last_error, accepted_at,
			created_at, updated_at
		FROM ap_outbound_follows
		WHERE local_actor_id = $1
		ORDER BY created_at DESC`, localActorID)
	if err != nil {
		return nil, fmt.Errorf("list outbound follows: %w", err)
	}
	defer rows.Close()
	follows := []OutboundFollow{}
	for rows.Next() {
		follow, err := scanOutboundFollow(rows)
		if err != nil {
			return nil, err
		}
		follows = append(follows, *follow)
	}
	return follows, rows.Err()
}

func (r *OutboundFollowRepo) ListAcceptedRemoteActorURIs(ctx context.Context, localActorID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT remote_actor_uri
		FROM ap_outbound_follows
		WHERE local_actor_id = $1 AND status = 'accepted'
		ORDER BY accepted_at DESC, remote_actor_uri`, localActorID)
	if err != nil {
		return nil, fmt.Errorf("list accepted outbound follows: %w", err)
	}
	defer rows.Close()
	actorURIs := []string{}
	for rows.Next() {
		var actorURI string
		if err := rows.Scan(&actorURI); err != nil {
			return nil, fmt.Errorf("scan accepted outbound follow: %w", err)
		}
		actorURIs = append(actorURIs, actorURI)
	}
	return actorURIs, rows.Err()
}

func (r *OutboundFollowRepo) DeleteOwned(ctx context.Context, localActorID, id string) (bool, error) {
	result, err := r.pool.Exec(ctx, `DELETE FROM ap_outbound_follows WHERE id = $1 AND local_actor_id = $2`, id, localActorID)
	if err != nil {
		return false, fmt.Errorf("delete outbound follow: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOutboundFollow(row rowScanner) (*OutboundFollow, error) {
	var follow OutboundFollow
	if err := row.Scan(
		&follow.ID, &follow.LocalActorID, &follow.RemoteActorURI, &follow.RemoteInboxURI,
		&follow.ActivityID, &follow.Status, &follow.LastDeliveryAt, &follow.LastError,
		&follow.AcceptedAt, &follow.CreatedAt, &follow.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &follow, nil
}
