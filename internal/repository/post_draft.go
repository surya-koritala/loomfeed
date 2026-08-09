package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostDraftRepo owns in-progress post drafts per user. Mirrors the
// four-tab Submit shape (text / image / link / poll); type-specific
// extras like image_urls and poll options live in metadata.
type PostDraftRepo struct {
	pool *pgxpool.Pool
}

func NewPostDraftRepo(pool *pgxpool.Pool) *PostDraftRepo {
	return &PostDraftRepo{pool: pool}
}

type PostDraft struct {
	ID          string         `json:"id"`
	OwnerID     string         `json:"owner_id"`
	CommunityID *string        `json:"community_id,omitempty"`
	PostType    string         `json:"post_type"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	URL         string         `json:"url"`
	Tags        []string       `json:"tags"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type PostDraftInput struct {
	CommunityID *string
	PostType    string
	Title       string
	Body        string
	URL         string
	Tags        []string
	Metadata    map[string]any
}

func (r *PostDraftRepo) Create(ctx context.Context, ownerID string, in PostDraftInput) (*PostDraft, error) {
	metaJSON, err := marshalMeta(in.Metadata)
	if err != nil {
		return nil, err
	}
	var d PostDraft
	var commOut *string
	var meta []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO post_drafts (owner_id, community_id, post_type, title, body, url, tags, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, owner_id, community_id, post_type, title, body, url, tags, metadata, created_at, updated_at`,
		ownerID, in.CommunityID, in.PostType, in.Title, in.Body, in.URL, in.Tags, metaJSON,
	).Scan(&d.ID, &d.OwnerID, &commOut, &d.PostType, &d.Title, &d.Body, &d.URL, &d.Tags, &meta, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}
	d.CommunityID = commOut
	d.Metadata = unmarshalMeta(meta)
	return &d, nil
}

func (r *PostDraftRepo) Update(ctx context.Context, id string, in PostDraftInput) error {
	metaJSON, err := marshalMeta(in.Metadata)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE post_drafts
		SET community_id = $2, post_type = $3, title = $4, body = $5, url = $6, tags = $7, metadata = $8, updated_at = NOW()
		WHERE id = $1`,
		id, in.CommunityID, in.PostType, in.Title, in.Body, in.URL, in.Tags, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("update draft: %w", err)
	}
	return nil
}

func (r *PostDraftRepo) Get(ctx context.Context, id string) (*PostDraft, error) {
	var d PostDraft
	var commOut *string
	var meta []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, community_id, post_type, title, body, url, tags, metadata, created_at, updated_at
		FROM post_drafts WHERE id = $1`, id,
	).Scan(&d.ID, &d.OwnerID, &commOut, &d.PostType, &d.Title, &d.Body, &d.URL, &d.Tags, &meta, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get draft: %w", err)
	}
	d.CommunityID = commOut
	d.Metadata = unmarshalMeta(meta)
	return &d, nil
}

func (r *PostDraftRepo) ListByOwner(ctx context.Context, ownerID string) ([]PostDraft, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, community_id, post_type, title, body, url, tags, metadata, created_at, updated_at
		FROM post_drafts
		WHERE owner_id = $1
		ORDER BY updated_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	defer rows.Close()
	out := []PostDraft{}
	for rows.Next() {
		var d PostDraft
		var commOut *string
		var meta []byte
		if err := rows.Scan(&d.ID, &d.OwnerID, &commOut, &d.PostType, &d.Title, &d.Body, &d.URL, &d.Tags, &meta, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan draft: %w", err)
		}
		d.CommunityID = commOut
		d.Metadata = unmarshalMeta(meta)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostDraftRepo) OwnerOf(ctx context.Context, id string) (string, error) {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT owner_id FROM post_drafts WHERE id = $1`, id).Scan(&owner)
	return owner, err
}

func (r *PostDraftRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM post_drafts WHERE id = $1`, id)
	return err
}

func marshalMeta(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}

func unmarshalMeta(b []byte) map[string]any {
	if len(b) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	return m
}
