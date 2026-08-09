package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Endorsement represents a capability endorsement between participants.
type Endorsement struct {
	EndorserID   string    `json:"endorser_id"`
	EndorserName string    `json:"endorser_name,omitempty"`
	EndorsedID   string    `json:"endorsed_id"`
	Capability   string    `json:"capability"`
	CreatedAt    time.Time `json:"created_at"`
}

// EndorsementRepo handles database operations for endorsements.
type EndorsementRepo struct {
	pool *pgxpool.Pool
}

// NewEndorsementRepo creates a new EndorsementRepo.
func NewEndorsementRepo(pool *pgxpool.Pool) *EndorsementRepo {
	return &EndorsementRepo{pool: pool}
}

// Endorse adds an endorsement (INSERT ON CONFLICT DO NOTHING).
func (r *EndorsementRepo) Endorse(ctx context.Context, endorserID, endorsedID, capability string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO endorsements (endorser_id, endorsed_id, capability)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		endorserID, endorsedID, capability)
	if err != nil {
		return fmt.Errorf("endorse: %w", err)
	}
	return nil
}

// Unendorse removes an endorsement.
func (r *EndorsementRepo) Unendorse(ctx context.Context, endorserID, endorsedID, capability string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM endorsements
		WHERE endorser_id = $1 AND endorsed_id = $2 AND capability = $3`,
		endorserID, endorsedID, capability)
	if err != nil {
		return fmt.Errorf("unendorse: %w", err)
	}
	return nil
}

// ListForAgent returns all endorsements for an agent.
func (r *EndorsementRepo) ListForAgent(ctx context.Context, agentID string) ([]Endorsement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.endorser_id, COALESCE(p.display_name, '') as endorser_name,
		       e.endorsed_id, e.capability, e.created_at
		FROM endorsements e
		LEFT JOIN participants p ON p.id = e.endorser_id
		WHERE e.endorsed_id = $1
		ORDER BY e.created_at DESC`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("list endorsements: %w", err)
	}
	defer rows.Close()

	var endorsements []Endorsement
	for rows.Next() {
		var e Endorsement
		if err := rows.Scan(&e.EndorserID, &e.EndorserName, &e.EndorsedID, &e.Capability, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan endorsement: %w", err)
		}
		endorsements = append(endorsements, e)
	}
	return endorsements, rows.Err()
}

// CountByCapability returns a map of capability -> count of endorsements for an agent.
func (r *EndorsementRepo) CountByCapability(ctx context.Context, agentID string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT capability, COUNT(*) as cnt
		FROM endorsements
		WHERE endorsed_id = $1
		GROUP BY capability
		ORDER BY cnt DESC`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("count endorsements by capability: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var cap string
		var cnt int
		if err := rows.Scan(&cap, &cnt); err != nil {
			return nil, fmt.Errorf("scan endorsement count: %w", err)
		}
		counts[cap] = cnt
	}
	return counts, rows.Err()
}
