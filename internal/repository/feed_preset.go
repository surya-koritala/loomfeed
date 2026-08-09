package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FeedPresetRepo owns named feed configurations per user. Each preset
// is a (sort, post_type, scope, community_slug) quad; the feed URL is
// built from that on the client side.
type FeedPresetRepo struct {
	pool *pgxpool.Pool
}

func NewFeedPresetRepo(pool *pgxpool.Pool) *FeedPresetRepo {
	return &FeedPresetRepo{pool: pool}
}

type FeedPreset struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	Name          string    `json:"name"`
	Sort          string    `json:"sort"`
	PostType      string    `json:"post_type"`
	Scope         string    `json:"scope"`
	CommunitySlug string    `json:"community_slug"`
	Position      int       `json:"position"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Create inserts a new preset at the end of the owner's list.
func (r *FeedPresetRepo) Create(ctx context.Context, ownerID, name, sort, postType, scope, communitySlug string) (*FeedPreset, error) {
	var p FeedPreset
	err := r.pool.QueryRow(ctx, `
        INSERT INTO feed_presets (owner_id, name, sort, post_type, scope, community_slug, position)
        VALUES ($1, $2, $3, $4, $5, $6,
            COALESCE((SELECT MAX(position) + 1 FROM feed_presets WHERE owner_id = $1), 0))
        RETURNING id, owner_id, name, sort, post_type, scope, community_slug, position, created_at, updated_at`,
		ownerID, name, sort, postType, scope, communitySlug).
		Scan(&p.ID, &p.OwnerID, &p.Name, &p.Sort, &p.PostType, &p.Scope, &p.CommunitySlug, &p.Position, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create preset: %w", err)
	}
	return &p, nil
}

// ListByOwner returns every preset for a user in sort order.
func (r *FeedPresetRepo) ListByOwner(ctx context.Context, ownerID string) ([]FeedPreset, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT id, owner_id, name, sort, post_type, scope, community_slug, position, created_at, updated_at
        FROM feed_presets
        WHERE owner_id = $1
        ORDER BY position, created_at`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()
	out := []FeedPreset{}
	for rows.Next() {
		var p FeedPreset
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Sort, &p.PostType, &p.Scope, &p.CommunitySlug, &p.Position, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update modifies a preset's mutable fields. Caller must verify
// ownership before calling.
func (r *FeedPresetRepo) Update(ctx context.Context, id, name, sort, postType, scope, communitySlug string) error {
	_, err := r.pool.Exec(ctx, `
        UPDATE feed_presets
        SET name = $2, sort = $3, post_type = $4, scope = $5, community_slug = $6, updated_at = NOW()
        WHERE id = $1`,
		id, name, sort, postType, scope, communitySlug)
	if err != nil {
		return fmt.Errorf("update preset: %w", err)
	}
	return nil
}

// Delete removes a preset. Caller must verify ownership.
func (r *FeedPresetRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM feed_presets WHERE id = $1`, id)
	return err
}

// OwnerOf returns the owner_id of a preset, for the auth check.
func (r *FeedPresetRepo) OwnerOf(ctx context.Context, id string) (string, error) {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT owner_id FROM feed_presets WHERE id = $1`, id).Scan(&owner)
	if err != nil {
		return "", err
	}
	return owner, nil
}
