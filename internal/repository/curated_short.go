package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CuratedShort mirrors the curated_shorts table. Serves as both the
// DB row and the JSON shape returned by feed + admin endpoints.
type CuratedShort struct {
	ID                  string     `json:"id"`
	Platform            string     `json:"platform"`
	PlatformVideoID     string     `json:"platform_video_id"`
	Title               string     `json:"title"`
	CreatorName         string     `json:"creator_name"`
	CreatorURL          string     `json:"creator_url"`
	Category            string     `json:"category"`
	EmbedURL            string     `json:"embed_url"`
	WatchURL            string     `json:"watch_url"`
	ThumbnailURL        string     `json:"thumbnail_url"`
	DurationSec         int        `json:"duration_sec"`
	ViewCount           int64      `json:"view_count"`
	AIScore             float64    `json:"ai_score"`
	AIRationale         string     `json:"ai_rationale"`
	Status              string     `json:"status"`
	ReviewedByID        *string    `json:"reviewed_by_id,omitempty"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty"`
	PlatformPublishedAt *time.Time `json:"platform_published_at,omitempty"`
	CuratedAt           time.Time  `json:"curated_at"`
}

// CuratedShortRepo persists curated-shorts rows + the minimal queries
// the feed + admin pages need. We keep the surface narrow: upsert on
// ingest, feed-list for public /shorts, pending-list for admins, and
// a status mutator for approve/reject actions.
type CuratedShortRepo struct {
	pool *pgxpool.Pool
}

func NewCuratedShortRepo(pool *pgxpool.Pool) *CuratedShortRepo {
	return &CuratedShortRepo{pool: pool}
}

// Upsert inserts a new pending row, or refreshes the statistics on
// re-ingest of a video we've already seen. Idempotent — safe to run
// the refresh job repeatedly without duplicating rows.
//
// On conflict we update view_count + ai_score + ai_rationale but do
// NOT touch status, so a previously-rejected video stays rejected
// even if the LLM loves it today.
func (r *CuratedShortRepo) Upsert(ctx context.Context, s *CuratedShort) error {
	_, err := r.pool.Exec(ctx, `
        INSERT INTO curated_shorts (
            platform, platform_video_id, title, creator_name, creator_url,
            category, embed_url, watch_url, thumbnail_url,
            duration_sec, view_count, ai_score, ai_rationale,
            status, platform_published_at
        ) VALUES (
            $1, $2, $3, $4, $5,
            $6, $7, $8, $9,
            $10, $11, $12, $13,
            'pending', $14
        )
        ON CONFLICT (platform, platform_video_id) DO UPDATE SET
            view_count   = EXCLUDED.view_count,
            ai_score     = EXCLUDED.ai_score,
            ai_rationale = EXCLUDED.ai_rationale,
            title        = EXCLUDED.title`,
		s.Platform, s.PlatformVideoID, s.Title, s.CreatorName, s.CreatorURL,
		s.Category, s.EmbedURL, s.WatchURL, s.ThumbnailURL,
		s.DurationSec, s.ViewCount, s.AIScore, s.AIRationale,
		s.PlatformPublishedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert curated short: %w", err)
	}
	return nil
}

const curatedShortsCols = `
    id, platform, platform_video_id, title, creator_name, creator_url,
    category, embed_url, watch_url, thumbnail_url,
    duration_sec, view_count, ai_score, ai_rationale,
    status, reviewed_by_id, reviewed_at,
    platform_published_at, curated_at
`

func scanCuratedShort(row pgx.Row) (*CuratedShort, error) {
	var s CuratedShort
	err := row.Scan(
		&s.ID, &s.Platform, &s.PlatformVideoID, &s.Title, &s.CreatorName, &s.CreatorURL,
		&s.Category, &s.EmbedURL, &s.WatchURL, &s.ThumbnailURL,
		&s.DurationSec, &s.ViewCount, &s.AIScore, &s.AIRationale,
		&s.Status, &s.ReviewedByID, &s.ReviewedAt,
		&s.PlatformPublishedAt, &s.CuratedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListFeed returns approved shorts for the public /shorts route.
// Empty category means "all approved." Newest curated_at first so
// the moderator's work shows up at the top of the feed immediately.
func (r *CuratedShortRepo) ListFeed(ctx context.Context, category string, limit, offset int) ([]CuratedShort, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var rows pgx.Rows
	var err error
	if category == "" {
		rows, err = r.pool.Query(ctx, `
            SELECT`+curatedShortsCols+`
            FROM curated_shorts
            WHERE status = 'approved'
            ORDER BY curated_at DESC
            LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
            SELECT`+curatedShortsCols+`
            FROM curated_shorts
            WHERE status = 'approved' AND category = $1
            ORDER BY curated_at DESC
            LIMIT $2 OFFSET $3`, category, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list curated feed: %w", err)
	}
	defer rows.Close()
	return collectCuratedShorts(rows)
}

// ListPending returns queue ordered by LLM score. Admin UI only.
func (r *CuratedShortRepo) ListPending(ctx context.Context, limit int) ([]CuratedShort, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
        SELECT`+curatedShortsCols+`
        FROM curated_shorts
        WHERE status = 'pending'
        ORDER BY ai_score DESC, curated_at DESC
        LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending curated: %w", err)
	}
	defer rows.Close()
	return collectCuratedShorts(rows)
}

func collectCuratedShorts(rows pgx.Rows) ([]CuratedShort, error) {
	out := []CuratedShort{}
	for rows.Next() {
		s, err := scanCuratedShort(rows)
		if err != nil {
			return nil, fmt.Errorf("scan curated short: %w", err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// SetStatus flips a row between pending / approved / rejected and
// stamps the reviewer. Caller must have already checked admin.
func (r *CuratedShortRepo) SetStatus(ctx context.Context, id, status, reviewerID string) error {
	if status != "pending" && status != "approved" && status != "rejected" && status != "expired" {
		return fmt.Errorf("invalid curated status %q", status)
	}
	_, err := r.pool.Exec(ctx, `
        UPDATE curated_shorts
        SET status = $2, reviewed_by_id = $3, reviewed_at = NOW()
        WHERE id = $1`, id, status, reviewerID)
	if err != nil {
		return fmt.Errorf("set curated status: %w", err)
	}
	return nil
}

// GetByID is used by the admin approval endpoints to echo back the
// row after mutation.
func (r *CuratedShortRepo) GetByID(ctx context.Context, id string) (*CuratedShort, error) {
	row := r.pool.QueryRow(ctx, `SELECT`+curatedShortsCols+`FROM curated_shorts WHERE id = $1`, id)
	return scanCuratedShort(row)
}

// RejectAllPending flips every currently-pending row to 'rejected'
// in one statement. Used by the admin "purge queue" endpoint when a
// scoring-rule change has invalidated everything in the existing
// pool. Returns the number of rows affected.
func (r *CuratedShortRepo) RejectAllPending(ctx context.Context, reviewerID string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
        UPDATE curated_shorts
        SET status = 'rejected', reviewed_by_id = $1, reviewed_at = NOW()
        WHERE status = 'pending'`, reviewerID)
	if err != nil {
		return 0, fmt.Errorf("reject all pending: %w", err)
	}
	return tag.RowsAffected(), nil
}
