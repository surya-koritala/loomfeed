package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// SearchRepo handles full-text search operations.
type SearchRepo struct {
	pool *pgxpool.Pool
}

// NewSearchRepo creates a new SearchRepo.
func NewSearchRepo(pool *pgxpool.Pool) *SearchRepo {
	return &SearchRepo{pool: pool}
}

// SearchPosts performs a full-text search over posts using PostgreSQL tsvector.
// Results are ranked by relevance then vote score. Supports prefix matching via :*.
func (r *SearchRepo) SearchPosts(ctx context.Context, query string, limit, offset int) ([]models.PostWithAuthor, int, error) {
	// Build a valid tsquery: join words with & operator, prefix-match on last word.
	// e.g. "legacy text" -> "legacy & text:*"
	words := strings.Fields(query)
	if len(words) == 0 {
		return []models.PostWithAuthor{}, 0, nil
	}
	words[len(words)-1] = words[len(words)-1] + ":*"
	tsQuery := strings.Join(words, " & ")

	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM posts WHERE search_vector @@ to_tsquery('english', $1) AND deleted_at IS NULL`,
		tsQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	if total == 0 {
		return []models.PostWithAuthor{}, 0, nil
	}

	rows, err := r.pool.Query(ctx, postJoinSelect+`
	WHERE p.search_vector @@ to_tsquery('english', $1)
	  AND p.deleted_at IS NULL
	ORDER BY ts_rank(p.search_vector, to_tsquery('english', $1)) DESC, p.vote_score DESC
	LIMIT $2 OFFSET $3`,
		tsQuery, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search posts: %w", err)
	}
	defer rows.Close()

	var results []models.PostWithAuthor
	for rows.Next() {
		p, err := scanPostWithAuthor(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, p)
	}
	return results, total, rows.Err()
}
