package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentCapability represents a capability registered by an agent.
type AgentCapability struct {
	ID           string          `json:"id"`
	AgentID      string          `json:"agent_id"`
	Capability   string          `json:"capability"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	EndpointURL  string          `json:"endpoint_url,omitempty"`
	IsVerified   bool            `json:"is_verified"`
	UsageCount   int             `json:"usage_count"`
	AvgRating    float64         `json:"avg_rating"`
	CreatedAt    time.Time       `json:"created_at"`
}

// AgentCapabilityWithAgent extends AgentCapability with agent profile data.
type AgentCapabilityWithAgent struct {
	AgentCapability
	AgentName     string  `json:"agent_name"`
	AgentType     string  `json:"agent_type"`
	TrustScore    float64 `json:"trust_score"`
	ModelProvider string  `json:"model_provider,omitempty"`
	ModelName     string  `json:"model_name,omitempty"`
}

// AgentCapabilityRepo handles database operations for agent capabilities.
type AgentCapabilityRepo struct {
	pool *pgxpool.Pool
}

// NewAgentCapabilityRepo creates a new AgentCapabilityRepo.
func NewAgentCapabilityRepo(pool *pgxpool.Pool) *AgentCapabilityRepo {
	return &AgentCapabilityRepo{pool: pool}
}

// Register upserts a capability for the given agent.
func (r *AgentCapabilityRepo) Register(ctx context.Context, agentID, capability, description string, inputSchema, outputSchema json.RawMessage, endpointURL string) (*AgentCapability, error) {
	// Enforce max 20 capabilities per agent.
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_capabilities WHERE agent_id = $1`, agentID,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("count agent capabilities: %w", err)
	}

	// Allow upsert if already registered, but block new ones over limit
	var existingCount int
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_capabilities WHERE agent_id = $1 AND capability = $2`,
		agentID, capability,
	).Scan(&existingCount)
	if err != nil {
		return nil, fmt.Errorf("check existing capability: %w", err)
	}
	if existingCount == 0 && count >= 20 {
		return nil, fmt.Errorf("capability limit reached: max 20 per agent")
	}

	var cap AgentCapability
	err = r.pool.QueryRow(ctx,
		`INSERT INTO agent_capabilities (agent_id, capability, description, input_schema, output_schema, endpoint_url)
         VALUES ($1, $2, $3, $4, $5, $6)
         ON CONFLICT (agent_id, capability) DO UPDATE SET
           description = EXCLUDED.description,
           input_schema = EXCLUDED.input_schema,
           output_schema = EXCLUDED.output_schema,
           endpoint_url = EXCLUDED.endpoint_url
         RETURNING id, agent_id, capability, COALESCE(description, ''), input_schema, output_schema, COALESCE(endpoint_url, ''), is_verified, usage_count, avg_rating, created_at`,
		agentID, capability, description, inputSchema, outputSchema, endpointURL,
	).Scan(&cap.ID, &cap.AgentID, &cap.Capability, &cap.Description,
		&cap.InputSchema, &cap.OutputSchema, &cap.EndpointURL,
		&cap.IsVerified, &cap.UsageCount, &cap.AvgRating, &cap.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("register agent capability: %w", err)
	}
	return &cap, nil
}

// Unregister removes a capability for the given agent.
func (r *AgentCapabilityRepo) Unregister(ctx context.Context, agentID, capability string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM agent_capabilities WHERE agent_id = $1 AND capability = $2`,
		agentID, capability)
	if err != nil {
		return fmt.Errorf("unregister agent capability: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("capability not found")
	}
	return nil
}

// GetByAgent returns all capabilities for a given agent.
func (r *AgentCapabilityRepo) GetByAgent(ctx context.Context, agentID string) ([]AgentCapability, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, agent_id, capability, COALESCE(description, ''), input_schema, output_schema,
		        COALESCE(endpoint_url, ''), is_verified, usage_count, avg_rating, created_at
		 FROM agent_capabilities
		 WHERE agent_id = $1
		 ORDER BY capability`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent capabilities: %w", err)
	}
	defer rows.Close()

	var caps []AgentCapability
	for rows.Next() {
		var c AgentCapability
		if err := rows.Scan(&c.ID, &c.AgentID, &c.Capability, &c.Description,
			&c.InputSchema, &c.OutputSchema, &c.EndpointURL,
			&c.IsVerified, &c.UsageCount, &c.AvgRating, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent capability: %w", err)
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

// Search finds agents by capability with optional filters.
func (r *AgentCapabilityRepo) Search(ctx context.Context, capability string, minRating float64, verifiedOnly bool, limit, offset int) ([]AgentCapabilityWithAgent, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Count total matches
	countQuery := `SELECT COUNT(*) FROM agent_capabilities ac
	               JOIN participants p ON p.id = ac.agent_id
	               WHERE ($1 = '' OR LOWER(ac.capability) = LOWER($1))
	                 AND ac.avg_rating >= $2
	                 AND ($3 = false OR ac.is_verified = true)`
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, capability, minRating, verifiedOnly).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count capabilities: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT ac.id, ac.agent_id, ac.capability, COALESCE(ac.description, ''),
		        ac.input_schema, ac.output_schema, COALESCE(ac.endpoint_url, ''),
		        ac.is_verified, ac.usage_count, ac.avg_rating, ac.created_at,
		        p.display_name, p.type, p.trust_score,
		        COALESCE(ai.model_provider, ''), COALESCE(ai.model_name, '')
		 FROM agent_capabilities ac
		 JOIN participants p ON p.id = ac.agent_id
		 LEFT JOIN agent_identities ai ON ai.participant_id = ac.agent_id
		 WHERE ($1 = '' OR LOWER(ac.capability) = LOWER($1))
		   AND ac.avg_rating >= $2
		   AND ($3 = false OR ac.is_verified = true)
		 ORDER BY ac.usage_count DESC, ac.avg_rating DESC
		 LIMIT $4 OFFSET $5`,
		capability, minRating, verifiedOnly, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search capabilities: %w", err)
	}
	defer rows.Close()

	var results []AgentCapabilityWithAgent
	for rows.Next() {
		var c AgentCapabilityWithAgent
		if err := rows.Scan(
			&c.ID, &c.AgentID, &c.Capability, &c.Description,
			&c.InputSchema, &c.OutputSchema, &c.EndpointURL,
			&c.IsVerified, &c.UsageCount, &c.AvgRating, &c.CreatedAt,
			&c.AgentName, &c.AgentType, &c.TrustScore,
			&c.ModelProvider, &c.ModelName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan capability search: %w", err)
		}
		results = append(results, c)
	}
	return results, total, rows.Err()
}

// IncrementUsage bumps the usage counter for a capability.
func (r *AgentCapabilityRepo) IncrementUsage(ctx context.Context, capabilityID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_capabilities SET usage_count = usage_count + 1 WHERE id = $1`,
		capabilityID)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("capability not found")
	}
	return nil
}

// Rate updates the average rating using a running average.
func (r *AgentCapabilityRepo) Rate(ctx context.Context, capabilityID string, rating float64) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be between 0 and 5")
	}
	// Running average: new_avg = (old_avg * count + rating) / (count + 1)
	// Also increment usage_count since a rating implies usage.
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_capabilities
		 SET avg_rating = (avg_rating * usage_count + $2) / (usage_count + 1),
		     usage_count = usage_count + 1
		 WHERE id = $1`,
		capabilityID, rating)
	if err != nil {
		return fmt.Errorf("rate capability: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("capability not found")
	}
	return nil
}

// GetByID returns a single capability by its ID.
func (r *AgentCapabilityRepo) GetByID(ctx context.Context, id string) (*AgentCapabilityWithAgent, error) {
	var c AgentCapabilityWithAgent
	err := r.pool.QueryRow(ctx,
		`SELECT ac.id, ac.agent_id, ac.capability, COALESCE(ac.description, ''),
		        ac.input_schema, ac.output_schema, COALESCE(ac.endpoint_url, ''),
		        ac.is_verified, ac.usage_count, ac.avg_rating, ac.created_at,
		        p.display_name, p.type, p.trust_score,
		        COALESCE(ai.model_provider, ''), COALESCE(ai.model_name, '')
		 FROM agent_capabilities ac
		 JOIN participants p ON p.id = ac.agent_id
		 LEFT JOIN agent_identities ai ON ai.participant_id = ac.agent_id
		 WHERE ac.id = $1`,
		id,
	).Scan(
		&c.ID, &c.AgentID, &c.Capability, &c.Description,
		&c.InputSchema, &c.OutputSchema, &c.EndpointURL,
		&c.IsVerified, &c.UsageCount, &c.AvgRating, &c.CreatedAt,
		&c.AgentName, &c.AgentType, &c.TrustScore,
		&c.ModelProvider, &c.ModelName,
	)
	if err != nil {
		return nil, fmt.Errorf("get capability: %w", err)
	}
	return &c, nil
}
