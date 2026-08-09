package curatedshorts

import (
	"context"
	"log/slog"

	"github.com/surya-koritala/loomfeed/internal/repository"
)

// Curator orchestrates a full refresh: for every configured category,
// walk its seed queries, fetch YouTube shorts, score each with the
// LLM, and upsert to the queue. All failures are per-video — one bad
// request doesn't abort the batch.
type Curator struct {
	youtube *YouTubeClient
	scorer  *Scorer
	repo    *repository.CuratedShortRepo
	// Minimum LLM score a video needs to clear before we even queue
	// it. 0.55 keeps the pending queue tight — below that is rarely
	// worth the moderator's time.
	minScore float64
}

// RefreshResult counts what happened per refresh. Admin endpoint
// returns this directly as the response body.
type RefreshResult struct {
	Fetched       int `json:"fetched"`
	Scored        int `json:"scored"`
	Queued        int `json:"queued"`
	BelowMinScore int `json:"below_min_score"`
	Failed        int `json:"failed"`
}

func NewCurator(youtube *YouTubeClient, scorer *Scorer, repo *repository.CuratedShortRepo) *Curator {
	return &Curator{
		youtube: youtube,
		scorer:  scorer,
		repo:    repo,
		// 0.55 — bumped up after a first run showed the LLM was
		// approving a lot of "Top 5 AI tools" clickbait at scores
		// 0.30-0.45. With the stricter system prompt, the floor for
		// substantive content lands around 0.6-0.7, so 0.55 keeps the
		// queue lean and high-quality. Tune SetMinScore() if needed.
		minScore: 0.55,
	}
}

// SetMinScore lets callers override the default threshold (e.g. for
// a stricter refresh once the system has been running for a while).
func (c *Curator) SetMinScore(s float64) { c.minScore = s }

// Refresh runs one full ingest + score pass across every category.
// Designed to be called by an admin endpoint, not a hot path — takes
// tens of seconds because it serializes per-video LLM calls.
func (c *Curator) Refresh(ctx context.Context) RefreshResult {
	var total RefreshResult
	for _, cat := range Categories {
		r := c.RefreshCategory(ctx, cat)
		total.Fetched += r.Fetched
		total.Scored += r.Scored
		total.Queued += r.Queued
		total.BelowMinScore += r.BelowMinScore
		total.Failed += r.Failed
	}
	return total
}

// RefreshCategory runs one category's worth of ingest. Exposed
// separately so a future admin UI could refresh a single lane.
func (c *Curator) RefreshCategory(ctx context.Context, cat Category) RefreshResult {
	var r RefreshResult
	if !c.youtube.Enabled() {
		slog.Warn("curatedshorts: youtube client disabled (missing API key)")
		return r
	}

	seen := make(map[string]struct{})
	for _, q := range cat.SearchQueries {
		if ctx.Err() != nil {
			return r
		}
		videos, err := c.youtube.Search(ctx, q, 25)
		if err != nil {
			slog.Warn("curatedshorts: youtube search failed",
				"category", cat.Slug, "query", q, "err", err)
			r.Failed++
			continue
		}
		r.Fetched += len(videos)

		for _, v := range videos {
			if ctx.Err() != nil {
				return r
			}
			// Dedupe across queries in the same category — different
			// queries often surface the same hot video twice.
			key := v.Platform + ":" + v.PlatformID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			decision := c.scorer.Score(ctx, v, cat.DisplayName)
			r.Scored++

			if decision.Score < c.minScore {
				r.BelowMinScore++
				continue
			}

			row := &repository.CuratedShort{
				Platform:        v.Platform,
				PlatformVideoID: v.PlatformID,
				Title:           v.Title,
				CreatorName:     v.CreatorName,
				CreatorURL:      v.CreatorURL,
				Category:        cat.Slug,
				EmbedURL:        v.EmbedURL,
				WatchURL:        v.WatchURL,
				ThumbnailURL:    v.ThumbnailURL,
				DurationSec:     v.DurationSec,
				ViewCount:       v.ViewCount,
				AIScore:         decision.Score,
				AIRationale:     decision.Rationale,
			}
			if !v.PublishedAt.IsZero() {
				pub := v.PublishedAt
				row.PlatformPublishedAt = &pub
			}

			if err := c.repo.Upsert(ctx, row); err != nil {
				slog.Warn("curatedshorts: upsert failed",
					"category", cat.Slug, "video", v.PlatformID, "err", err)
				r.Failed++
				continue
			}
			r.Queued++
		}
	}
	slog.Info("curatedshorts: category refresh done",
		"category", cat.Slug,
		"fetched", r.Fetched, "scored", r.Scored,
		"queued", r.Queued, "below_min", r.BelowMinScore, "failed", r.Failed)
	return r
}
