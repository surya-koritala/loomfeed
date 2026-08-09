package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BYOKAgent mirrors a row in byok_agents. encrypted_api_key is intentionally
// exposed in the struct so the runtime can pass it to the vault for
// decryption; never surface it in HTTP responses.
type BYOKAgent struct {
	ID                 string    `json:"id"`
	OwnerID            string    `json:"owner_id"`
	AgentParticipantID string    `json:"agent_participant_id"`
	Provider           string    `json:"provider"`
	Model              string    `json:"model"`
	EncryptedAPIKey    string    `json:"-"`
	PersonaPrompt      string    `json:"persona_prompt"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Populated on list queries by joining participants.
	DisplayName string `json:"display_name,omitempty"`
}

type BYOKAgentRepo struct {
	pool *pgxpool.Pool
}

func NewBYOKAgentRepo(pool *pgxpool.Pool) *BYOKAgentRepo {
	return &BYOKAgentRepo{pool: pool}
}

type CreateBYOKInput struct {
	OwnerID            string
	AgentParticipantID string
	Provider           string
	Model              string
	EncryptedAPIKey    string
	PersonaPrompt      string
}

func (r *BYOKAgentRepo) Create(ctx context.Context, in CreateBYOKInput) (*BYOKAgent, error) {
	var a BYOKAgent
	err := r.pool.QueryRow(ctx, `
		INSERT INTO byok_agents
		  (owner_id, agent_participant_id, provider, model, encrypted_api_key, persona_prompt)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, owner_id, agent_participant_id, provider, model,
		          encrypted_api_key, persona_prompt, enabled, created_at, updated_at`,
		in.OwnerID, in.AgentParticipantID, in.Provider, in.Model,
		in.EncryptedAPIKey, in.PersonaPrompt,
	).Scan(
		&a.ID, &a.OwnerID, &a.AgentParticipantID, &a.Provider, &a.Model,
		&a.EncryptedAPIKey, &a.PersonaPrompt, &a.Enabled, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert byok_agent: %w", err)
	}
	return &a, nil
}

// ListByOwner returns every BYOK agent the user owns, with the agent's
// display name joined in for UI convenience.
func (r *BYOKAgentRepo) ListByOwner(ctx context.Context, ownerID string) ([]BYOKAgent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.owner_id, b.agent_participant_id, b.provider, b.model,
		       b.encrypted_api_key, b.persona_prompt, b.enabled,
		       b.created_at, b.updated_at, p.display_name
		FROM byok_agents b
		JOIN participants p ON p.id = b.agent_participant_id
		WHERE b.owner_id = $1
		ORDER BY b.created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list byok_agents: %w", err)
	}
	defer rows.Close()

	out := []BYOKAgent{}
	for rows.Next() {
		var a BYOKAgent
		if err := rows.Scan(
			&a.ID, &a.OwnerID, &a.AgentParticipantID, &a.Provider, &a.Model,
			&a.EncryptedAPIKey, &a.PersonaPrompt, &a.Enabled,
			&a.CreatedAt, &a.UpdatedAt, &a.DisplayName,
		); err != nil {
			return nil, fmt.Errorf("scan byok_agent: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetByID returns a single BYOK agent if it exists and belongs to ownerID.
// Returning nil + nil error means "not found for this owner".
func (r *BYOKAgentRepo) GetByID(ctx context.Context, id, ownerID string) (*BYOKAgent, error) {
	var a BYOKAgent
	err := r.pool.QueryRow(ctx, `
		SELECT b.id, b.owner_id, b.agent_participant_id, b.provider, b.model,
		       b.encrypted_api_key, b.persona_prompt, b.enabled,
		       b.created_at, b.updated_at, p.display_name
		FROM byok_agents b
		JOIN participants p ON p.id = b.agent_participant_id
		WHERE b.id = $1 AND b.owner_id = $2`, id, ownerID,
	).Scan(
		&a.ID, &a.OwnerID, &a.AgentParticipantID, &a.Provider, &a.Model,
		&a.EncryptedAPIKey, &a.PersonaPrompt, &a.Enabled,
		&a.CreatedAt, &a.UpdatedAt, &a.DisplayName,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetByAgentParticipantID is used by the summon path: given the agent's
// participant id, load its BYOK config (verifying owner).
func (r *BYOKAgentRepo) GetByAgentParticipantID(ctx context.Context, agentPID, ownerID string) (*BYOKAgent, error) {
	var a BYOKAgent
	err := r.pool.QueryRow(ctx, `
		SELECT b.id, b.owner_id, b.agent_participant_id, b.provider, b.model,
		       b.encrypted_api_key, b.persona_prompt, b.enabled,
		       b.created_at, b.updated_at, p.display_name
		FROM byok_agents b
		JOIN participants p ON p.id = b.agent_participant_id
		WHERE b.agent_participant_id = $1 AND b.owner_id = $2`, agentPID, ownerID,
	).Scan(
		&a.ID, &a.OwnerID, &a.AgentParticipantID, &a.Provider, &a.Model,
		&a.EncryptedAPIKey, &a.PersonaPrompt, &a.Enabled,
		&a.CreatedAt, &a.UpdatedAt, &a.DisplayName,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Delete removes the byok_agent row; the ON DELETE CASCADE on
// agent_participant_id removes the participant as well.
func (r *BYOKAgentRepo) Delete(ctx context.Context, id, ownerID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM byok_agents WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return fmt.Errorf("delete byok_agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

// SetEnabled toggles the enabled flag.
func (r *BYOKAgentRepo) SetEnabled(ctx context.Context, id, ownerID string, enabled bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE byok_agents SET enabled = $1, updated_at = NOW() WHERE id = $2 AND owner_id = $3`,
		enabled, id, ownerID)
	return err
}
