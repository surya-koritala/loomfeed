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
func (r *SearchRepo) SearchPosts(ctx context.Context, query string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
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
		`SELECT COUNT(*) FROM posts WHERE search_vector @@ to_tsquery('english', $1) AND deleted_at IS NULL AND quarantined = FALSE`,
		tsQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	if total == 0 {
		return []models.PostWithAuthor{}, 0, nil
	}

	args := []any{tsQuery}
	cursorClause := ""
	offsetClause := ""
	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		args = append(args, cursor[0])
		cursorParam := fmt.Sprintf("$%d", len(args))
		cursorClause = fmt.Sprintf(`
	  AND (ts_rank(p.search_vector, to_tsquery('english', $1)), p.vote_score, p.id) < (
		SELECT ts_rank(ap.search_vector, to_tsquery('english', $1)), ap.vote_score, ap.id
		FROM posts ap WHERE ap.id = %s
	  )`, cursorParam)
	}
	args = append(args, limit)
	limitParam := fmt.Sprintf("$%d", len(args))
	if !useCursor {
		args = append(args, offset)
		offsetClause = fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, postJoinSelect+`
	WHERE p.search_vector @@ to_tsquery('english', $1)
	  AND p.deleted_at IS NULL
	  AND p.quarantined = FALSE`+cursorClause+`
	ORDER BY ts_rank(p.search_vector, to_tsquery('english', $1)) DESC, p.vote_score DESC, p.id DESC
	LIMIT `+limitParam+offsetClause,
		args...)
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
