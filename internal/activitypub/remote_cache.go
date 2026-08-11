package activitypub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RemoteActorCache is the durable counterpart to FetchActor's short-lived
// process cache. It prevents every API replica and restart from repeating
// WebFinger/actor discovery for a recently seen remote identity.
type RemoteActorCache struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewRemoteActorCache(pool *pgxpool.Pool, ttl time.Duration) *RemoteActorCache {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &RemoteActorCache{pool: pool, ttl: ttl}
}

func (c *RemoteActorCache) Put(ctx context.Context, acct string, actor *RemoteActor) error {
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return fmt.Errorf("cache remote actor: actor id is required")
	}
	document, err := json.Marshal(actor)
	if err != nil {
		return fmt.Errorf("cache remote actor: marshal document: %w", err)
	}
	acct = strings.ToLower(strings.TrimSpace(acct))
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cache remote actor: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A WebFinger acct may legitimately move to a new canonical actor URL.
	// Remove only that stale alias before upserting the refreshed document.
	if acct != "" {
		if _, err := tx.Exec(ctx, `DELETE FROM ap_remote_actor_cache WHERE acct = $1 AND actor_uri <> $2`, acct, actor.ID); err != nil {
			return fmt.Errorf("cache remote actor: replace moved acct: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ap_remote_actor_cache (actor_uri, acct, document, fetched_at, expires_at)
		VALUES ($1, NULLIF($2, ''), $3, NOW(), $4)
		ON CONFLICT (actor_uri) DO UPDATE SET
			acct = COALESCE(EXCLUDED.acct, ap_remote_actor_cache.acct),
			document = EXCLUDED.document,
			fetched_at = NOW(),
			expires_at = EXCLUDED.expires_at`,
		actor.ID, acct, document, time.Now().Add(c.ttl),
	)
	if err != nil {
		return fmt.Errorf("cache remote actor: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cache remote actor: commit: %w", err)
	}
	return nil
}

func (c *RemoteActorCache) Get(ctx context.Context, actorURI string) (*RemoteActor, bool, error) {
	return c.get(ctx, `actor_uri = $1`, strings.TrimSpace(actorURI))
}

func (c *RemoteActorCache) GetByAcct(ctx context.Context, acct string) (*RemoteActor, bool, error) {
	return c.get(ctx, `acct = $1`, strings.ToLower(strings.TrimSpace(acct)))
}

func (c *RemoteActorCache) get(ctx context.Context, predicate, value string) (*RemoteActor, bool, error) {
	if value == "" {
		return nil, false, nil
	}
	var document []byte
	err := c.pool.QueryRow(ctx, `
		SELECT document
		FROM ap_remote_actor_cache
		WHERE `+predicate+` AND expires_at > NOW()`, value).Scan(&document)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read remote actor cache: %w", err)
	}
	var actor RemoteActor
	if err := json.Unmarshal(document, &actor); err != nil {
		return nil, false, fmt.Errorf("read remote actor cache: decode document: %w", err)
	}
	return &actor, true, nil
}
