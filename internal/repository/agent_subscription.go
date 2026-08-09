package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentSubscription represents an agent's event subscription.
type AgentSubscription struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	SubscriptionType string    `json:"subscription_type"`
	FilterValue      string    `json:"filter_value"`
	WebhookURL       *string   `json:"webhook_url,omitempty"`
	IsActive         bool      `json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
}

// AgentSubscriptionRepo handles database operations for agent subscriptions.
type AgentSubscriptionRepo struct {
	pool *pgxpool.Pool
}

// NewAgentSubscriptionRepo creates a new AgentSubscriptionRepo.
func NewAgentSubscriptionRepo(pool *pgxpool.Pool) *AgentSubscriptionRepo {
	return &AgentSubscriptionRepo{pool: pool}
}

// Create inserts a new agent subscription after enforcing the per-agent limit.
func (r *AgentSubscriptionRepo) Create(ctx context.Context, agentID, subType, filterValue string, webhookURL *string) (*AgentSubscription, error) {
	// Enforce max 50 subscriptions per agent.
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_subscriptions WHERE agent_id = $1`, agentID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count agent subscriptions: %w", err)
	}
	if count >= 50 {
		return nil, fmt.Errorf("subscription limit reached: max 50 per agent")
	}

	var sub AgentSubscription
	err = r.pool.QueryRow(ctx,
		`INSERT INTO agent_subscriptions (agent_id, subscription_type, filter_value, webhook_url)
         VALUES ($1, $2, $3, $4)
         RETURNING id, agent_id, subscription_type, filter_value, webhook_url, is_active, created_at`,
		agentID, subType, filterValue, webhookURL,
	).Scan(&sub.ID, &sub.AgentID, &sub.SubscriptionType, &sub.FilterValue, &sub.WebhookURL, &sub.IsActive, &sub.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create agent subscription: %w", err)
	}
	return &sub, nil
}

// ListByAgent returns all subscriptions for a given agent.
func (r *AgentSubscriptionRepo) ListByAgent(ctx context.Context, agentID string) ([]AgentSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, subscription_type, filter_value, webhook_url, is_active, created_at
         FROM agent_subscriptions
         WHERE agent_id = $1
         ORDER BY created_at DESC`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []AgentSubscription
	for rows.Next() {
		var s AgentSubscription
		if err := rows.Scan(&s.ID, &s.AgentID, &s.SubscriptionType, &s.FilterValue, &s.WebhookURL, &s.IsActive, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// Delete removes a subscription owned by the agent.
func (r *AgentSubscriptionRepo) Delete(ctx context.Context, id, agentID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM agent_subscriptions WHERE id = $1 AND agent_id = $2`,
		id, agentID)
	if err != nil {
		return fmt.Errorf("delete agent subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("subscription not found or not owned by agent")
	}
	return nil
}

// FindMatching returns all active subscriptions that match the given type and filter value.
func (r *AgentSubscriptionRepo) FindMatching(ctx context.Context, subType, filterValue string) ([]AgentSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, subscription_type, filter_value, webhook_url, is_active, created_at
         FROM agent_subscriptions
         WHERE is_active = TRUE
           AND subscription_type = $1
           AND LOWER(filter_value) = LOWER($2)`,
		subType, filterValue)
	if err != nil {
		return nil, fmt.Errorf("find matching subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []AgentSubscription
	for rows.Next() {
		var s AgentSubscription
		if err := rows.Scan(&s.ID, &s.AgentID, &s.SubscriptionType, &s.FilterValue, &s.WebhookURL, &s.IsActive, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan matching subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// FindKeywordMatches returns all active keyword subscriptions where the keyword appears
// in the given text (case-insensitive). This performs the matching in the database.
func (r *AgentSubscriptionRepo) FindKeywordMatches(ctx context.Context, text string) ([]AgentSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, subscription_type, filter_value, webhook_url, is_active, created_at
         FROM agent_subscriptions
         WHERE is_active = TRUE
           AND subscription_type = 'keyword'
           AND LOWER($1) LIKE '%' || LOWER(filter_value) || '%'`,
		text)
	if err != nil {
		return nil, fmt.Errorf("find keyword matches: %w", err)
	}
	defer rows.Close()

	var subs []AgentSubscription
	for rows.Next() {
		var s AgentSubscription
		if err := rows.Scan(&s.ID, &s.AgentID, &s.SubscriptionType, &s.FilterValue, &s.WebhookURL, &s.IsActive, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan keyword subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}
