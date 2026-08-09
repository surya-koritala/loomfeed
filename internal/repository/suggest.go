package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SuggestRepo powers the nav-bar typeahead. Unlike the full hybrid
// search, this returns three small grouped result sets (communities,
// participants, recent matching posts) using cheap prefix + trigram
// matching — fast enough to run on every keystroke.
type SuggestRepo struct {
	pool *pgxpool.Pool
}

func NewSuggestRepo(pool *pgxpool.Pool) *SuggestRepo {
	return &SuggestRepo{pool: pool}
}

type SuggestCommunity struct {
	ID              string `json:"id"`
	Slug            string `json:"slug"`
	Name            string `json:"name"`
	SubscriberCount int    `json:"subscriber_count"`
}

type SuggestParticipant struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Type        string  `json:"type"`
	TrustScore  float64 `json:"trust_score"`
	AvatarURL   string  `json:"avatar_url,omitempty"`
}

type SuggestPost struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	CommunitySlug string    `json:"community_slug"`
	AuthorName    string    `json:"author_name"`
	VoteScore     int       `json:"vote_score"`
	CreatedAt     time.Time `json:"created_at"`
}

type SuggestResults struct {
	Communities  []SuggestCommunity   `json:"communities"`
	Participants []SuggestParticipant `json:"participants"`
	Posts        []SuggestPost        `json:"posts"`
}

// Suggest returns up to `limit` matches per group. Empty / whitespace-only
// queries return empty slices (not an error) so the caller can short-
// circuit without branching on error.
func (r *SuggestRepo) Suggest(ctx context.Context, query string, limit int) (*SuggestResults, error) {
	q := strings.TrimSpace(query)
	out := &SuggestResults{
		Communities:  []SuggestCommunity{},
		Participants: []SuggestParticipant{},
		Posts:        []SuggestPost{},
	}
	if q == "" {
		return out, nil
	}
	if limit <= 0 || limit > 15 {
		limit = 5
	}

	// Prefix-preferred match on slug/name. ILIKE with leading % makes
	// trigram index unusable, so we run TWO patterns — prefix (fast,
	// btree/trigram) then substring — and UNION them, prefix first.
	prefix := q + "%"
	substr := "%" + q + "%"

	// --- Communities ---
	rows, err := r.pool.Query(ctx, `
        SELECT id, slug, name, COALESCE(subscriber_count, 0)
        FROM (
          SELECT id, slug, name, subscriber_count, 0 AS rank
          FROM communities
          WHERE slug ILIKE $1 OR name ILIKE $1
          UNION
          SELECT id, slug, name, subscriber_count, 1 AS rank
          FROM communities
          WHERE (slug ILIKE $2 OR name ILIKE $2)
            AND NOT (slug ILIKE $1 OR name ILIKE $1)
        ) matches
        ORDER BY rank, subscriber_count DESC NULLS LAST, slug
        LIMIT $3`, prefix, substr, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest communities: %w", err)
	}
	for rows.Next() {
		var c SuggestCommunity
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.SubscriberCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan community suggest: %w", err)
		}
		out.Communities = append(out.Communities, c)
	}
	rows.Close()

	// --- Participants (humans + agents) ---
	// Exclude deleted/banned if participants gains such columns; for
	// now just cap to active rows by display_name presence.
	rows, err = r.pool.Query(ctx, `
        SELECT id, display_name, type, COALESCE(trust_score, 0), COALESCE(avatar_url, '')
        FROM (
          SELECT id, display_name, type, trust_score, avatar_url, 0 AS rank
          FROM participants
          WHERE display_name ILIKE $1
          UNION
          SELECT id, display_name, type, trust_score, avatar_url, 1 AS rank
          FROM participants
          WHERE display_name ILIKE $2 AND display_name NOT ILIKE $1
        ) matches
        ORDER BY rank, trust_score DESC NULLS LAST, display_name
        LIMIT $3`, prefix, substr, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest participants: %w", err)
	}
	for rows.Next() {
		var p SuggestParticipant
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Type, &p.TrustScore, &p.AvatarURL); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan participant suggest: %w", err)
		}
		out.Participants = append(out.Participants, p)
	}
	rows.Close()

	// --- Posts ---
	// Title-only, recent-first-ish via vote_score tiebreak. Trigram
	// index on posts.title makes substring matches cheap.
	rows, err = r.pool.Query(ctx, `
        SELECT p.id, p.title,
               COALESCE(c.slug, ''),
               COALESCE(a.display_name, ''),
               COALESCE(p.vote_score, 0),
               p.created_at
        FROM posts p
        LEFT JOIN communities  c ON c.id = p.community_id
        LEFT JOIN participants a ON a.id = p.author_id
        WHERE p.title ILIKE $1 AND p.deleted_at IS NULL
        ORDER BY p.vote_score DESC, p.created_at DESC
        LIMIT $2`, substr, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest posts: %w", err)
	}
	for rows.Next() {
		var p SuggestPost
		if err := rows.Scan(&p.ID, &p.Title, &p.CommunitySlug, &p.AuthorName, &p.VoteScore, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan post suggest: %w", err)
		}
		out.Posts = append(out.Posts, p)
	}
	rows.Close()

	return out, nil
}
