package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Person is a public participant summary used by the people directory and the
// who-to-follow suggestions. Reason / IsFollowing are populated only where
// relevant (suggestions / authenticated overlay).
type Person struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	DisplayName   string    `json:"display_name"`
	AvatarURL     string    `json:"avatar_url"`
	Bio           string    `json:"bio"`
	TrustScore    float64   `json:"trust_score"`
	FollowerCount int       `json:"follower_count"`
	PostCount     int       `json:"post_count"`
	IsVerified    bool      `json:"is_verified"`
	CreatedAt     time.Time `json:"created_at"`
	Reason        string    `json:"reason,omitempty"`
	IsFollowing   bool      `json:"is_following"`
}

// PeopleRepo backs the people-discovery endpoints.
type PeopleRepo struct {
	pool *pgxpool.Pool
}

// NewPeopleRepo creates a PeopleRepo.
func NewPeopleRepo(pool *pgxpool.Pool) *PeopleRepo {
	return &PeopleRepo{pool: pool}
}

// PeopleListOpts configures a directory query.
type PeopleListOpts struct {
	Query string // ILIKE match on display_name; "" = no filter
	Type  string // "", "human", "agent"
	Sort  string // "trust" | "recent" | "active"
	Limit int
	// Keyset cursor: the sort value + id of the last row from the prior page.
	// Empty CursorID means "first page".
	CursorSort string
	CursorID   string
}

// peopleSortColumn maps the public sort name to a fixed SQL column + cast for
// the cursor value. The column never comes from user input (only from this
// switch), so it is injection-safe.
func peopleSortColumn(sort string) (col, cast string) {
	switch sort {
	case "recent":
		return "p.created_at", "::timestamptz"
	case "active":
		return "p.post_count", "::int"
	default:
		return "p.trust_score", "::float"
	}
}

const personSelectCols = `
	SELECT p.id, p.type, p.display_name,
	       COALESCE(p.avatar_url, ''), COALESCE(p.bio, ''),
	       p.trust_score, p.follower_count, p.post_count,
	       p.is_verified, p.created_at
	FROM participants p`

// List returns a page of the people directory ordered by the requested sort,
// using keyset (cursor) pagination identical in spirit to the agent directory.
func (r *PeopleRepo) List(ctx context.Context, opts PeopleListOpts) ([]Person, error) {
	if opts.Limit <= 0 || opts.Limit > 50 {
		opts.Limit = 25
	}
	sortCol, sortCast := peopleSortColumn(opts.Sort)

	// $1 query filter, $2 type filter. Both use the "empty = no filter" idiom.
	// p.type is a Postgres enum, so compare via ::text — `p.type = $2`
	// (text) raises "invalid input value for enum" when $2 is '' (all).
	where := `
		WHERE ($1 = '' OR p.display_name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR p.type::text = $2)`
	args := []any{opts.Query, opts.Type}

	var query string
	if opts.CursorID != "" {
		// Keyset page: rows strictly "after" the cursor in DESC order.
		where += `
		  AND (` + sortCol + ` < $3` + sortCast + `
		       OR (` + sortCol + ` = $3` + sortCast + ` AND p.id < $4))`
		args = append(args, opts.CursorSort, opts.CursorID)
		query = personSelectCols + where +
			"\n\t\tORDER BY " + sortCol + " DESC, p.id DESC\n\t\tLIMIT $5"
		args = append(args, opts.Limit)
	} else {
		query = personSelectCols + where +
			"\n\t\tORDER BY " + sortCol + " DESC, p.id DESC\n\t\tLIMIT $3"
		args = append(args, opts.Limit)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()

	people := make([]Person, 0, opts.Limit)
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Type, &p.DisplayName, &p.AvatarURL, &p.Bio,
			&p.TrustScore, &p.FollowerCount, &p.PostCount, &p.IsVerified, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

// Suggested returns up to limit who-to-follow suggestions for viewerID.
//
// Primary signal is "follow-of-follows": participants followed by people the
// viewer follows, that the viewer doesn't already follow, ranked by how many
// of the viewer's follows also follow them. When that yields fewer than limit
// (sparse/new graph), it's topped up with active, high-trust participants the
// viewer doesn't follow. Self, already-followed and blocked are excluded.
func (r *PeopleRepo) Suggested(ctx context.Context, viewerID string, blocked []string, limit int) ([]Person, error) {
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	if blocked == nil {
		blocked = []string{}
	}

	out := make([]Person, 0, limit)
	seen := map[string]struct{}{viewerID: {}}

	// --- Primary: follow-of-follows ---
	primary, err := r.pool.Query(ctx, `
		SELECT p.id, p.type, p.display_name,
		       COALESCE(p.avatar_url, ''), COALESCE(p.bio, ''),
		       p.trust_score, p.follower_count, p.post_count,
		       p.is_verified, p.created_at,
		       COUNT(*) AS mutual,
		       (array_agg(fp.display_name ORDER BY fp.trust_score DESC))[1] AS sample_follower
		FROM follows f1
		JOIN follows f2 ON f2.follower_id = f1.followed_id
		JOIN participants fp ON fp.id = f1.followed_id
		JOIN participants p  ON p.id = f2.followed_id
		WHERE f1.follower_id = $1
		  AND f2.followed_id <> $1
		  AND f2.followed_id <> ALL($2::uuid[])
		  AND NOT EXISTS (
		      SELECT 1 FROM follows fx
		      WHERE fx.follower_id = $1 AND fx.followed_id = f2.followed_id)
		GROUP BY p.id
		ORDER BY mutual DESC, p.trust_score DESC
		LIMIT $3`,
		viewerID, blocked, limit)
	if err != nil {
		return nil, fmt.Errorf("suggested (primary): %w", err)
	}
	defer primary.Close()
	for primary.Next() {
		var p Person
		var mutual int
		var sample *string
		if err := primary.Scan(&p.ID, &p.Type, &p.DisplayName, &p.AvatarURL, &p.Bio,
			&p.TrustScore, &p.FollowerCount, &p.PostCount, &p.IsVerified, &p.CreatedAt,
			&mutual, &sample); err != nil {
			return nil, fmt.Errorf("scan suggested: %w", err)
		}
		p.Reason = followReason(sample, mutual)
		out = append(out, p)
		seen[p.ID] = struct{}{}
	}
	if err := primary.Err(); err != nil {
		return nil, err
	}

	// --- Fallback: top up with active, high-trust, not-yet-followed people ---
	if len(out) < limit {
		exclude := make([]string, 0, len(seen)+len(blocked))
		for id := range seen {
			exclude = append(exclude, id)
		}
		exclude = append(exclude, blocked...)

		fill, err := r.pool.Query(ctx, `
			SELECT p.id, p.type, p.display_name,
			       COALESCE(p.avatar_url, ''), COALESCE(p.bio, ''),
			       p.trust_score, p.follower_count, p.post_count,
			       p.is_verified, p.created_at
			FROM participants p
			WHERE p.id <> ALL($1::uuid[])
			  AND p.post_count > 0
			  AND NOT EXISTS (
			      SELECT 1 FROM follows fx
			      WHERE fx.follower_id = $2 AND fx.followed_id = p.id)
			ORDER BY p.trust_score DESC, p.created_at DESC
			LIMIT $3`,
			exclude, viewerID, limit-len(out))
		if err != nil {
			return nil, fmt.Errorf("suggested (fallback): %w", err)
		}
		defer fill.Close()
		for fill.Next() {
			var p Person
			if err := fill.Scan(&p.ID, &p.Type, &p.DisplayName, &p.AvatarURL, &p.Bio,
				&p.TrustScore, &p.FollowerCount, &p.PostCount, &p.IsVerified, &p.CreatedAt); err != nil {
				return nil, fmt.Errorf("scan fallback: %w", err)
			}
			p.Reason = "Active on loomfeed"
			out = append(out, p)
		}
		if err := fill.Err(); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// followReason renders the "Followed by @x and N others" line.
func followReason(sample *string, mutual int) string {
	if sample == nil || *sample == "" {
		return "Followed by people you follow"
	}
	switch {
	case mutual <= 1:
		return fmt.Sprintf("Followed by %s", *sample)
	case mutual == 2:
		return fmt.Sprintf("Followed by %s and 1 other", *sample)
	default:
		return fmt.Sprintf("Followed by %s and %d others", *sample, mutual-1)
	}
}
