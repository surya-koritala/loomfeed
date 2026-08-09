package sports

import (
	"context"
	"log/slog"
	"time"

	"github.com/surya-koritala/loomfeed/internal/repository"
)

const (
	// liveInterval is the poll cadence while a match is in play or kicks
	// off within imminentWindow. Well inside football-data.org's free-tier
	// rate limit (10 req/min).
	liveInterval = time.Minute
	// idleInterval is the poll cadence outside match windows.
	idleInterval = 60 * time.Minute
	// imminentWindow is how far ahead a kickoff counts as imminent.
	imminentWindow = 2 * time.Hour
)

// Poller keeps sports_matches in sync with football-data.org and settles
// predictions as matches finish. It adapts its cadence: every minute around
// live matches, hourly otherwise.
type Poller struct {
	client *Client
	repo   *repository.SportsRepo
}

// NewPoller creates a Poller.
func NewPoller(client *Client, repo *repository.SportsRepo) *Poller {
	return &Poller{client: client, repo: repo}
}

// Run ticks immediately, then loops until ctx is done, sleeping liveInterval
// when a match is live or imminent and idleInterval otherwise.
func (p *Poller) Run(ctx context.Context) {
	for {
		p.tick(ctx)

		interval := idleInterval
		live, err := p.repo.LiveOrImminent(ctx, imminentWindow)
		if err != nil {
			// Fail open toward the short interval so a transient DB
			// error can't stall live-match updates for an hour.
			slog.Warn("sports poller: live check failed", "error", err)
			interval = liveInterval
		} else if live {
			interval = liveInterval
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// tick fetches the World Cup schedule, upserts every match, and settles
// FINISHED ones. All errors are logged and skipped (fail-open): a bad fetch
// or one bad row must never kill the polling loop.
func (p *Poller) tick(ctx context.Context) {
	matches, err := p.client.FetchWorldCupMatches(ctx)
	if err != nil {
		slog.Warn("sports poller: fetch failed", "error", err)
		return
	}

	for i := range matches {
		m := &matches[i]
		id, err := p.repo.UpsertMatch(ctx, m)
		if err != nil {
			slog.Warn("sports poller: upsert failed", "ext_id", m.ExtID, "error", err)
			continue
		}
		if m.Status == "FINISHED" {
			// Settle unconditionally rather than re-reading settled_at:
			// SettleMatch is idempotent by design (it only grades
			// predictions whose outcome IS NULL), so repeat calls on an
			// already-settled match are cheap no-ops.
			if err := p.repo.SettleMatch(ctx, id); err != nil {
				slog.Warn("sports poller: settle failed", "match_id", id, "ext_id", m.ExtID, "error", err)
			}
		}
	}
}
