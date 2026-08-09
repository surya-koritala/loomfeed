package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/models"
)

// APIKeyRepo handles database operations for API keys.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo creates a new APIKeyRepo.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

// Create inserts a new API key record with an optional prefix for fast lookup.
func (r *APIKeyRepo) Create(ctx context.Context, k *models.APIKey) (*models.APIKey, error) {
	var result models.APIKey
	err := r.pool.QueryRow(ctx, `
		INSERT INTO api_keys
		  (agent_id, key_hash, key_prefix, scopes, rate_limit, expires_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
		  id, agent_id, key_hash, scopes, rate_limit, expires_at, is_active, created_at`,
		k.AgentID,
		k.KeyHash,
		k.KeyPrefix,
		k.Scopes,
		k.RateLimit,
		k.ExpiresAt,
		k.IsActive,
	).Scan(
		&result.ID, &result.AgentID, &result.KeyHash, &result.Scopes,
		&result.RateLimit, &result.ExpiresAt, &result.IsActive, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert api_key: %w", err)
	}
	return &result, nil
}

// GetByPrefix finds an active API key by its plaintext prefix (O(1) lookup).
func (r *APIKeyRepo) GetByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	var k models.APIKey
	// LEFT JOIN: the owning agent's max_rpm rides along so the auth
	// middleware can enforce the per-agent budget without a second
	// query per cache fill.
	err := r.pool.QueryRow(ctx, `
		SELECT k.id, k.agent_id, k.key_hash, k.scopes, k.rate_limit,
		       k.expires_at, k.is_active, k.created_at,
		       COALESCE(ai.max_rpm, 0)
		FROM api_keys k
		LEFT JOIN agent_identities ai ON ai.participant_id = k.agent_id
		WHERE k.key_prefix = $1 AND k.is_active = TRUE AND k.expires_at > NOW()
		LIMIT 1`, prefix).Scan(
		&k.ID, &k.AgentID, &k.KeyHash, &k.Scopes,
		&k.RateLimit, &k.ExpiresAt, &k.IsActive, &k.CreatedAt,
		&k.AgentMaxRPM,
	)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// SetPrefix updates the key_prefix for a given key (used to backfill old keys).
func (r *APIKeyRepo) SetPrefix(ctx context.Context, keyID, prefix string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET key_prefix = $1 WHERE id = $2`, prefix, keyID)
	return err
}

// GetActiveByAgent returns all active, non-expired API keys for the given agent.
func (r *APIKeyRepo) GetActiveByAgent(ctx context.Context, agentID string) ([]models.APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, key_hash, scopes, rate_limit, expires_at, is_active, created_at
		FROM api_keys
		WHERE agent_id = $1 AND is_active = TRUE AND expires_at > NOW()
		ORDER BY created_at DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get active api keys by agent: %w", err)
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(
			&k.ID, &k.AgentID, &k.KeyHash, &k.Scopes,
			&k.RateLimit, &k.ExpiresAt, &k.IsActive, &k.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning api_key row: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api_key rows: %w", err)
	}

	return keys, nil
}

// GetAllActiveWithoutPrefix returns active, non-expired API keys
// that have no key_prefix recorded yet. Only used as the fallback
// path in the auth middleware: the fast prefix lookup handles every
// key created after the prefix-indexing rollout, this slow path
// covers only legacy keys whose prefix hasn't been backfilled yet.
//
// Without this filter, the fallback fetches *every* active key and
// bcrypt-compares each one — for ~80 keys at ~100ms each that's
// 8 seconds per invalid auth attempt. Cloudflare cuts the request
// past its stream window and the client sees a hang. With the
// filter, the fallback runs against a small (often zero) legacy
// set, so an invalid key returns its 401 in ms.
func (r *APIKeyRepo) GetAllActiveWithoutPrefix(ctx context.Context) ([]models.APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, agent_id, key_hash, scopes, rate_limit, expires_at, is_active, created_at
		FROM api_keys
		WHERE is_active = TRUE
		  AND expires_at > NOW()
		  AND (key_prefix IS NULL OR key_prefix = '')`)
	if err != nil {
		return nil, fmt.Errorf("get legacy api keys: %w", err)
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(
			&k.ID, &k.AgentID, &k.KeyHash, &k.Scopes,
			&k.RateLimit, &k.ExpiresAt, &k.IsActive, &k.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning api_key row: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api_key rows: %w", err)
	}
	return keys, nil
}

// GetAllActive returns all active, non-expired API keys across all agents.
// Used by the API key auth middleware to validate incoming keys.
func (r *APIKeyRepo) GetAllActive(ctx context.Context) ([]models.APIKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT k.id, k.agent_id, k.key_hash, k.scopes, k.rate_limit,
		       k.expires_at, k.is_active, k.created_at,
		       COALESCE(ai.max_rpm, 0)
		FROM api_keys k
		LEFT JOIN agent_identities ai ON ai.participant_id = k.agent_id
		WHERE k.is_active = TRUE AND k.expires_at > NOW()`)
	if err != nil {
		return nil, fmt.Errorf("get all active api keys: %w", err)
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(
			&k.ID, &k.AgentID, &k.KeyHash, &k.Scopes,
			&k.RateLimit, &k.ExpiresAt, &k.IsActive, &k.CreatedAt,
			&k.AgentMaxRPM,
		); err != nil {
			return nil, fmt.Errorf("scanning api_key row: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api_key rows: %w", err)
	}

	return keys, nil
}

// RevokeAllForAgent deactivates all API keys for the given agent.
// Called when generating a new key to ensure only one key is active at a time.
func (r *APIKeyRepo) RevokeAllForAgent(ctx context.Context, agentID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET is_active = FALSE, revoked_at = NOW(), revoke_reason = 'key_rotation' WHERE agent_id = $1 AND is_active = TRUE`, agentID)
	if err != nil {
		return fmt.Errorf("revoke all agent keys: %w", err)
	}
	return nil
}

// Revoke deactivates an API key with an audit trail.
func (r *APIKeyRepo) Revoke(ctx context.Context, keyID string) error {
	return r.RevokeWithReason(ctx, keyID, "", "manual_revocation")
}

// RevokeWithReason deactivates an API key and records who revoked it and why.
func (r *APIKeyRepo) RevokeWithReason(ctx context.Context, keyID, revokedBy, reason string) error {
	query := `UPDATE api_keys SET is_active = FALSE, revoked_at = NOW(), revoke_reason = $2`
	args := []any{keyID, reason}
	if revokedBy != "" {
		query += `, revoked_by = $3 WHERE id = $1`
		args = append(args, revokedBy)
	} else {
		query += ` WHERE id = $1`
	}
	_, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("revoke api_key: %w", err)
	}
	return nil
}

// UpdateLastUsed records the last time an API key was used (by prefix).
func (r *APIKeyRepo) UpdateLastUsed(ctx context.Context, prefix string) error {
	_, err := r.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = NOW() WHERE key_prefix = $1 AND is_active = TRUE`, prefix)
	return err
}
