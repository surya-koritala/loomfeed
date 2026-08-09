package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformStatsWorker maintains the platform_stats single-row
// snapshot used by /api/v1/stats. Without it, every cache miss
// fired 6 sequential COUNT(*) queries on tables with 46k+ rows —
// ~1.5s per cold call. Now /stats reads one indexed row.
//
// The background pass runs the same six counts but at a 5-minute
// cadence, off the request path. The numbers move slowly enough
// that nobody notices a 5-min lag. /api/v1/stats then becomes a
// single sub-millisecond SELECT.
type PlatformStatsWorker struct {
	pool *pgxpool.Pool
}

func NewPlatformStatsWorker(pool *pgxpool.Pool) *PlatformStatsWorker {
	return &PlatformStatsWorker{pool: pool}
}

func (w *PlatformStatsWorker) Run(ctx context.Context, interval time.Duration) {
	// Warm-up before first run so /stats has fresh data on a fresh
	// deploy. The migration seeds zeros — 5s warm-up means cold
	// hits between deploy and first refresh see zeros, which is
	// less misleading than stale numbers from a previous deploy.
	time.Sleep(5 * time.Second)
	w.refresh(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.refresh(ctx)
		}
	}
}

// refreshStatsSQL is one round-trip: subselect each count, write
// them all into the single platform_stats row.
const refreshStatsSQL = `
UPDATE platform_stats SET
    total_agents       = (SELECT COUNT(*) FROM participants WHERE type = 'agent'),
    total_communities  = (SELECT COUNT(*) FROM communities),
    total_posts        = (SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL),
    total_comments     = (SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL),
    agent_post_count   = (SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL AND author_type = 'agent'),
    agent_comment_count= (SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL AND author_type = 'agent'),
    refreshed_at       = NOW()
WHERE id = 1
`

func (w *PlatformStatsWorker) refresh(ctx context.Context) {
	start := time.Now()
	if _, err := w.pool.Exec(ctx, refreshStatsSQL); err != nil {
		slog.Error("platform_stats refresh failed", "error", err)
		return
	}
	slog.Info("platform_stats refresh ok", "elapsed", time.Since(start))
}
