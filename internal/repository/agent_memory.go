package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentMemoryEntry represents a single key-value pair in an agent's persistent memory.
type AgentMemoryEntry struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agent_id"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// AgentMemoryRepo handles database operations for agent memory.
type AgentMemoryRepo struct {
	pool *pgxpool.Pool
}

// NewAgentMemoryRepo creates a new AgentMemoryRepo.
func NewAgentMemoryRepo(pool *pgxpool.Pool) *AgentMemoryRepo {
	return &AgentMemoryRepo{pool: pool}
}

// Set upserts a key-value pair for the given agent. If the key already exists,
// the value and updated_at are updated.
func (r *AgentMemoryRepo) Set(ctx context.Context, agentID, key string, value json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO agent_memory (agent_id, key, value)
         VALUES ($1, $2, $3)
         ON CONFLICT (agent_id, key) DO UPDATE
         SET value = EXCLUDED.value, updated_at = NOW()`,
		agentID, key, value,
	)
	if err != nil {
		return fmt.Errorf("set agent memory: %w", err)
	}
	return nil
}

// Get retrieves a single key-value pair for the given agent.
func (r *AgentMemoryRepo) Get(ctx context.Context, agentID, key string) (*AgentMemoryEntry, error) {
	var entry AgentMemoryEntry
	err := r.pool.QueryRow(ctx,
		`SELECT id, agent_id, key, value, created_at, updated_at
         FROM agent_memory
         WHERE agent_id = $1 AND key = $2`,
		agentID, key,
	).Scan(&entry.ID, &entry.AgentID, &entry.Key, &entry.Value, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent memory: %w", err)
	}
	return &entry, nil
}

// List returns all key-value pairs for the given agent, optionally filtered by key prefix.
func (r *AgentMemoryRepo) List(ctx context.Context, agentID string, prefix string) ([]AgentMemoryEntry, error) {
	var rows pgx.Rows
	var err error

	if prefix != "" {
		rows, err = r.pool.Query(ctx,
			`SELECT id, agent_id, key, value, created_at, updated_at
             FROM agent_memory
             WHERE agent_id = $1 AND key LIKE $2
             ORDER BY key ASC`,
			agentID, prefix+"%",
		)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT id, agent_id, key, value, created_at, updated_at
             FROM agent_memory
             WHERE agent_id = $1
             ORDER BY key ASC`,
			agentID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list agent memory: %w", err)
	}
	defer rows.Close()

	var entries []AgentMemoryEntry
	for rows.Next() {
		var entry AgentMemoryEntry
		if err := rows.Scan(&entry.ID, &entry.AgentID, &entry.Key, &entry.Value, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent memory entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Count returns the number of keys stored for the given agent.
func (r *AgentMemoryRepo) Count(ctx context.Context, agentID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_memory WHERE agent_id = $1`,
		agentID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count agent memory: %w", err)
	}
	return count, nil
}

// Delete removes a single key-value pair for the given agent.
func (r *AgentMemoryRepo) Delete(ctx context.Context, agentID, key string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM agent_memory WHERE agent_id = $1 AND key = $2`,
		agentID, key,
	)
	if err != nil {
		return fmt.Errorf("delete agent memory: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key not found")
	}
	return nil
}

// DeleteAll removes all key-value pairs for the given agent.
func (r *AgentMemoryRepo) DeleteAll(ctx context.Context, agentID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM agent_memory WHERE agent_id = $1`,
		agentID,
	)
	if err != nil {
		return fmt.Errorf("delete all agent memory: %w", err)
	}
	return nil
}
