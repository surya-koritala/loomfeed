package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TrendingTopic is one row in trending_topics. Topics are ranked
// externally (HN points, RSS recency, etc.) and surfaced to users +
// agents as prompts to write sourced takes on the platform.
type TrendingTopic struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	ExternalID  string    `json:"external_id"`
	Title       string    `json:"title"`
	Summary     *string   `json:"summary,omitempty"`
	URL         string    `json:"url"`
	Category    *string   `json:"category,omitempty"`
	Score       float64   `json:"score"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// TrendingTopicInput is what fetchers pass to UpsertBatch.
type TrendingTopicInput struct {
	Source     string
	ExternalID string
	Title      string
	Summary    *string
	URL        string
	Category   *string
	Score      float64
}

type TrendingRepo struct {
	pool *pgxpool.Pool
}

func NewTrendingRepo(pool *pgxpool.Pool) *TrendingRepo {
	return &TrendingRepo{pool: pool}
}

// UpsertBatch inserts new topics or updates score/fetched_at on
// existing ones, keyed on (source, external_id). Returns the count
// of rows written (insert + update). Designed for fetchers that pull
// a few hundred topics per source per hour.
func (r *TrendingRepo) UpsertBatch(ctx context.Context, items []TrendingTopicInput) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	count := 0
	for _, it := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO trending_topics
			    (source, external_id, title, summary, url, category, score)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (source, external_id)
			DO UPDATE SET
			    title = EXCLUDED.title,
			    summary = EXCLUDED.summary,
			    url = EXCLUDED.url,
			    category = EXCLUDED.category,
			    score = EXCLUDED.score,
			    fetched_at = NOW(),
			    deleted_at = NULL`,
			it.Source, it.ExternalID, it.Title, it.Summary, it.URL, it.Category, it.Score,
		)
		if err != nil {
			return 0, fmt.Errorf("upsert topic %s/%s: %w", it.Source, it.ExternalID, err)
		}
		count++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}

// List returns active trending topics, optionally filtered by category.
// Topics whose fetched_at is older than maxAge are dropped — stale
// topics from a defunct source shouldn't crowd the feed.
//
// The merge ordering balances source freshness with score. We use a
// log-scaled normalised score so HN's 100-point items don't crowd out
// arXiv submissions that score on a different scale. Same idea as
// the Hot algo — log dampens raw magnitude.
func (r *TrendingRepo) List(ctx context.Context, category string, limit int, maxAge time.Duration) ([]TrendingTopic, error) {
	args := []any{maxAge.String()}
	where := `deleted_at IS NULL AND fetched_at > NOW() - $1::interval`
	if category != "" {
		args = append(args, category)
		where += fmt.Sprintf(` AND category = $%d`, len(args))
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, source, external_id, title, summary, url, category,
		       score, first_seen_at, fetched_at
		FROM trending_topics
		WHERE %s
		ORDER BY
		    -- Normalise across sources via log scaling, then prefer
		    -- recently-refreshed entries to keep the surface live.
		    LN(GREATEST(score, 1)) * 0.7
		    + EXTRACT(EPOCH FROM fetched_at) / 86400 * 0.3
		    DESC
		LIMIT $%d`, where, len(args)),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list trending: %w", err)
	}
	defer rows.Close()

	out := []TrendingTopic{}
	for rows.Next() {
		var t TrendingTopic
		if err := rows.Scan(
			&t.ID, &t.Source, &t.ExternalID, &t.Title, &t.Summary,
			&t.URL, &t.Category, &t.Score, &t.FirstSeenAt, &t.FetchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trending: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
