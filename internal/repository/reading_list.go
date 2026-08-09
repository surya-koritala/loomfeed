package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadingListRepo owns curated-post bundles. Each list has one owner
// (no collaborators in v1) and an ordered sequence of posts. Items are
// appended at MAX(position)+1; there's no reordering API in this pass.
type ReadingListRepo struct {
	pool *pgxpool.Pool
}

func NewReadingListRepo(pool *pgxpool.Pool) *ReadingListRepo {
	return &ReadingListRepo{pool: pool}
}

type ReadingList struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	OwnerName   string    `json:"owner_name,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public"`
	ItemCount   int       `json:"item_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ReadingListItem struct {
	ListID    string    `json:"list_id"`
	PostID    string    `json:"post_id"`
	Position  int       `json:"position"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`

	// Joined from posts for the list-view endpoint.
	PostTitle     string `json:"post_title,omitempty"`
	PostBody      string `json:"post_body,omitempty"`
	CommunitySlug string `json:"community_slug,omitempty"`
	AuthorName    string `json:"author_name,omitempty"`
	AuthorType    string `json:"author_type,omitempty"`
	VoteScore     int    `json:"vote_score"`
	CommentCount  int    `json:"comment_count"`
}

// Create inserts a new list. Title must be 1..120 chars (enforced by
// CHECK + handler). Description may be empty.
func (r *ReadingListRepo) Create(ctx context.Context, ownerID, title, description string, isPublic bool) (*ReadingList, error) {
	var l ReadingList
	err := r.pool.QueryRow(ctx, `
		INSERT INTO reading_lists (owner_id, title, description, is_public)
		VALUES ($1, $2, $3, $4)
		RETURNING id, owner_id, title, description, is_public, created_at, updated_at`,
		ownerID, title, description, isPublic,
	).Scan(&l.ID, &l.OwnerID, &l.Title, &l.Description, &l.IsPublic, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert reading list: %w", err)
	}
	return &l, nil
}

// Get fetches a list with its owner's display name and item count.
// Does NOT apply visibility rules — caller must check l.IsPublic vs
// the viewer's identity before returning to a non-owner.
func (r *ReadingListRepo) Get(ctx context.Context, id string) (*ReadingList, error) {
	var l ReadingList
	err := r.pool.QueryRow(ctx, `
		SELECT l.id, l.owner_id, p.display_name, l.title, l.description, l.is_public,
		       l.created_at, l.updated_at,
		       COALESCE((SELECT COUNT(*) FROM reading_list_items WHERE list_id = l.id), 0)
		FROM reading_lists l
		JOIN participants p ON p.id = l.owner_id
		WHERE l.id = $1`,
		id,
	).Scan(&l.ID, &l.OwnerID, &l.OwnerName, &l.Title, &l.Description, &l.IsPublic,
		&l.CreatedAt, &l.UpdatedAt, &l.ItemCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("list not found")
		}
		return nil, fmt.Errorf("get reading list: %w", err)
	}
	return &l, nil
}

// ListByOwner returns every list owned by a participant. Used for
// the /me/lists page and (public-only variant) profile views.
func (r *ReadingListRepo) ListByOwner(ctx context.Context, ownerID string, publicOnly bool) ([]ReadingList, error) {
	q := `
		SELECT l.id, l.owner_id, l.title, l.description, l.is_public,
		       l.created_at, l.updated_at,
		       COALESCE((SELECT COUNT(*) FROM reading_list_items WHERE list_id = l.id), 0)
		FROM reading_lists l
		WHERE l.owner_id = $1`
	if publicOnly {
		q += ` AND l.is_public = TRUE`
	}
	q += ` ORDER BY l.updated_at DESC LIMIT 100`

	rows, err := r.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list reading lists: %w", err)
	}
	defer rows.Close()

	var out []ReadingList
	for rows.Next() {
		var l ReadingList
		if err := rows.Scan(&l.ID, &l.OwnerID, &l.Title, &l.Description, &l.IsPublic,
			&l.CreatedAt, &l.UpdatedAt, &l.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if out == nil {
		out = []ReadingList{}
	}
	return out, rows.Err()
}

// Items returns the ordered posts in a list, with enough post +
// author metadata to render the /lists/[id] page without an N+1.
func (r *ReadingListRepo) Items(ctx context.Context, listID string) ([]ReadingListItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			i.list_id, i.post_id, i.position, i.note, i.created_at,
			COALESCE(p.title, '') AS post_title,
			COALESCE(p.body, '') AS post_body,
			COALESCE(c.slug, '') AS community_slug,
			COALESCE(part.display_name, '') AS author_name,
			COALESCE(part.type::text, '') AS author_type,
			COALESCE(p.vote_score, 0) AS vote_score,
			COALESCE(p.comment_count, 0) AS comment_count
		FROM reading_list_items i
		JOIN posts p ON p.id = i.post_id
		LEFT JOIN communities c ON c.id = p.community_id
		LEFT JOIN participants part ON part.id = p.author_id
		WHERE i.list_id = $1
		ORDER BY i.position ASC, i.created_at ASC`,
		listID,
	)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()

	var out []ReadingListItem
	for rows.Next() {
		var it ReadingListItem
		if err := rows.Scan(
			&it.ListID, &it.PostID, &it.Position, &it.Note, &it.CreatedAt,
			&it.PostTitle, &it.PostBody, &it.CommunitySlug,
			&it.AuthorName, &it.AuthorType, &it.VoteScore, &it.CommentCount,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if out == nil {
		out = []ReadingListItem{}
	}
	return out, rows.Err()
}

// AddItem appends a post to a list. Position is assigned as
// MAX(position)+1 atomically to avoid a race on concurrent appends.
// Caller must verify ownership.
func (r *ReadingListRepo) AddItem(ctx context.Context, listID, postID, note string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reading_list_items (list_id, post_id, position, note)
		VALUES ($1, $2, COALESCE((SELECT MAX(position) + 1 FROM reading_list_items WHERE list_id = $1), 0), $3)
		ON CONFLICT (list_id, post_id) DO NOTHING`,
		listID, postID, note,
	)
	if err != nil {
		return fmt.Errorf("add list item: %w", err)
	}
	return nil
}

// RemoveItem pulls a post out of a list. No-op if not present.
func (r *ReadingListRepo) RemoveItem(ctx context.Context, listID, postID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM reading_list_items WHERE list_id = $1 AND post_id = $2`,
		listID, postID,
	)
	if err != nil {
		return fmt.Errorf("remove list item: %w", err)
	}
	return nil
}

// Update mutates title / description / privacy. Fields not set are
// unchanged.
func (r *ReadingListRepo) Update(ctx context.Context, id string, title, description *string, isPublic *bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE reading_lists
		SET title = COALESCE($2, title),
		    description = COALESCE($3, description),
		    is_public = COALESCE($4, is_public),
		    updated_at = NOW()
		WHERE id = $1`,
		id, title, description, isPublic,
	)
	if err != nil {
		return fmt.Errorf("update list: %w", err)
	}
	return nil
}

// Delete removes a list (cascades to items).
func (r *ReadingListRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM reading_lists WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete list: %w", err)
	}
	return nil
}
