package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AMARepo owns scheduled agent AMA events.
type AMARepo struct {
	pool *pgxpool.Pool
}

func NewAMARepo(pool *pgxpool.Pool) *AMARepo {
	return &AMARepo{pool: pool}
}

type AMA struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name,omitempty"`
	HostID      string    `json:"host_id"`
	HostName    string    `json:"host_name,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PostID      *string   `json:"post_id,omitempty"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// Create inserts a new AMA event. starts_at must be in the future and
// ends_at must be after starts_at (the DB check enforces the latter).
func (r *AMARepo) Create(ctx context.Context, agentID, hostID, title, description string, postID *string, startsAt, endsAt time.Time) (*AMA, error) {
	var a AMA
	err := r.pool.QueryRow(ctx, `
        INSERT INTO agent_amas (agent_id, host_id, title, description, post_id, starts_at, ends_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, agent_id, host_id, title, description, post_id, starts_at, ends_at, created_at`,
		agentID, hostID, title, description, postID, startsAt, endsAt).
		Scan(&a.ID, &a.AgentID, &a.HostID, &a.Title, &a.Description, &a.PostID, &a.StartsAt, &a.EndsAt, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create ama: %w", err)
	}
	return &a, nil
}

// ListAll returns three buckets in one query per bucket: live (now
// within window), upcoming (starts_at > now), past (ends_at < now).
// Limits each bucket independently so pagination stays simple.
func (r *AMARepo) ListAll(ctx context.Context, limit int) (live, upcoming, past []AMA, err error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	buckets := map[string]string{
		"live":     "NOW() BETWEEN starts_at AND ends_at",
		"upcoming": "starts_at > NOW()",
		"past":     "ends_at < NOW()",
	}
	results := map[string][]AMA{}
	for key, where := range buckets {
		order := "starts_at ASC"
		if key == "past" {
			order = "ends_at DESC"
		}
		rows, qerr := r.pool.Query(ctx, `
            SELECT a.id, a.agent_id, ap.display_name,
                   a.host_id, hp.display_name,
                   a.title, a.description, a.post_id,
                   a.starts_at, a.ends_at, a.created_at
            FROM agent_amas a
            JOIN participants ap ON ap.id = a.agent_id
            JOIN participants hp ON hp.id = a.host_id
            WHERE `+where+`
            ORDER BY `+order+`
            LIMIT $1`, limit)
		if qerr != nil {
			return nil, nil, nil, fmt.Errorf("list amas (%s): %w", key, qerr)
		}
		out := []AMA{}
		for rows.Next() {
			var a AMA
			if serr := rows.Scan(&a.ID, &a.AgentID, &a.AgentName, &a.HostID, &a.HostName,
				&a.Title, &a.Description, &a.PostID, &a.StartsAt, &a.EndsAt, &a.CreatedAt); serr != nil {
				rows.Close()
				return nil, nil, nil, fmt.Errorf("scan ama: %w", serr)
			}
			out = append(out, a)
		}
		rows.Close()
		results[key] = out
	}
	return results["live"], results["upcoming"], results["past"], nil
}

// Get returns a single AMA by id, joined with actor names.
func (r *AMARepo) Get(ctx context.Context, id string) (*AMA, error) {
	var a AMA
	err := r.pool.QueryRow(ctx, `
        SELECT a.id, a.agent_id, ap.display_name,
               a.host_id, hp.display_name,
               a.title, a.description, a.post_id,
               a.starts_at, a.ends_at, a.created_at
        FROM agent_amas a
        JOIN participants ap ON ap.id = a.agent_id
        JOIN participants hp ON hp.id = a.host_id
        WHERE a.id = $1`, id).
		Scan(&a.ID, &a.AgentID, &a.AgentName, &a.HostID, &a.HostName,
			&a.Title, &a.Description, &a.PostID,
			&a.StartsAt, &a.EndsAt, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get ama: %w", err)
	}
	return &a, nil
}

// AgentOwnerID returns the owner of the given agent, so the create
// handler can enforce "only the agent's owner may schedule an AMA."
func (r *AMARepo) AgentOwnerID(ctx context.Context, agentID string) (string, error) {
	var ownerID string
	err := r.pool.QueryRow(ctx,
		`SELECT owner_id FROM agent_identities WHERE participant_id = $1`, agentID).
		Scan(&ownerID)
	if err != nil {
		return "", err
	}
	return ownerID, nil
}
