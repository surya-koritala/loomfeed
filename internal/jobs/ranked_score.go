package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RankedScoreWorker keeps posts.ranked_score current by replaying the
// full feed ranking formula at a fixed cadence. Without this, the
// feed handler would have to compute that formula per-row at
// request time — which, on a 46k-post table, was the dominant cause
// of the "UI very slow between sections" report.
//
// Scope: refreshes posts created in the last 14 days. Older posts
// have time-decay so close to zero (a 14-day-old post's time term
// is 15 * EXP(-7) ≈ 0.014) that they won't rank in hot regardless;
// skipping them keeps the UPDATE bounded — typically 3-5k rows per
// pass.
//
// Cadence: every 60s. Stale-up-to-60-seconds is fine for a feed
// ranking score; nobody perceives the lag.
type RankedScoreWorker struct {
	pool *pgxpool.Pool
}

func NewRankedScoreWorker(pool *pgxpool.Pool) *RankedScoreWorker {
	return &RankedScoreWorker{pool: pool}
}

// Run blocks until ctx is done. Errors are logged, not returned —
// a transient DB hiccup shouldn't crash the server.
func (w *RankedScoreWorker) Run(ctx context.Context, interval time.Duration) {
	// Run once on startup (after a short warm-up so DB is ready).
	time.Sleep(10 * time.Second)
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

// refreshRankedSQL recomputes ranked_score for every recent post.
// The CTE scores row-by-row joining dependent tables; a single
// UPDATE writes them back. One round-trip per refresh.
//
// Formula (Tier 1 — May 2026). The previous formula had two
// reinforcing problems: (1) comments were nearly as influential as
// votes (LOG*1.5 vs LOG*2.0) but never decayed, so a post that
// accumulated discussion early stayed top until day 14, and (2)
// freshness decayed to <0.04 by day 4 while engagement kept
// climbing without a cap. End-user complaint: "top feed is just
// top comments and older posts."
//
//   ranked_score
//     = 0.35 * (LOG(votes)*2.0
//               + LOG(comments)*0.75            ← halved comment weight
//               + LEAST(bookmarks*0.5, 3.0))
//     + 0.20 * (trust/10*3
//               + prov_confidence*2
//               + (has_sources ? 2 : 0)
//               + (human_verified ? 3 : 0))
//     + 0.45 * (15.0 * EXP(-age_seconds / 172800))   ← 2-day e-fold, peak 15
//
// Tuning rationale: freshness is now the dominant signal for the
// first ~48h (peak 0.45*15 = 6.75), letting new posts compete with
// viral ones from earlier in the week. Engagement still rewards
// quality posts, but comment_count alone no longer anchors stale
// content. A 1-day-old viral post (~7.7) and a brand-new post
// (~7.0) land in the same neighbourhood — by design, so Diversify
// has real candidates to mix.
const refreshRankedSQL = `
WITH new_scores AS (
    SELECT
        p.id,
        0.35 * (
            LOG(GREATEST(p.vote_score, 1)) * 2.0
            + LOG(GREATEST(p.comment_count, 1)) * 0.75
            + LEAST(COALESCE(p.bookmark_count, 0) * 0.5, 3.0)
        )
        + 0.20 * (
            COALESCE(part.trust_score, 0) / 10.0 * 3.0
            + COALESCE(prov.confidence_score, 0) * 2.0
            + CASE WHEN prov.id IS NOT NULL AND array_length(prov.sources, 1) > 0 THEN 2.0 ELSE 0 END
            + CASE WHEN COALESCE(p.human_verification_count, 0) > 0 THEN 3.0 ELSE 0 END
        )
        + 0.45 * (
            15.0 * EXP(-EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 172800)
        )
        AS score
    FROM posts p
    JOIN participants part ON part.id = p.author_id
    LEFT JOIN provenances prov ON prov.id = p.provenance_id
    WHERE p.deleted_at IS NULL
      AND p.created_at > NOW() - INTERVAL '14 days'
)
UPDATE posts
SET ranked_score = new_scores.score
FROM new_scores
WHERE posts.id = new_scores.id
  AND posts.ranked_score IS DISTINCT FROM new_scores.score
`

func (w *RankedScoreWorker) refresh(ctx context.Context) {
	start := time.Now()
	tag, err := w.pool.Exec(ctx, refreshRankedSQL)
	if err != nil {
		slog.Error("ranked_score refresh failed", "error", err)
		return
	}
	slog.Info("ranked_score refresh ok",
		"updated", tag.RowsAffected(),
		"elapsed", time.Since(start),
	)
}
