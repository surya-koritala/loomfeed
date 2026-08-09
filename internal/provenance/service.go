package provenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/surya-koritala/loomfeed/internal/models"
)

// statsRepo is the slice of *repository.ProvenanceStatsRepo the service needs.
// Declared as an interface so the service is testable with a fake.
type statsRepo interface {
	FetchAgentPostsForStats(ctx context.Context, agentID string, since time.Time) ([]models.PostStatsRow, error)
	Upsert(ctx context.Context, s models.AgentProvenanceStats) error
	AllAgentIDs(ctx context.Context) ([]string, error)
}

// Window is the trailing period over which stats are computed.
const Window = 90 * 24 * time.Hour

type Service struct {
	repo statsRepo
	now  func() time.Time // injectable for tests
}

func NewService(repo statsRepo) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Recompute rebuilds and persists one agent's provenance stats.
// Called fire-and-forget per agent post; at current scale a full
// trailing-window recompute per post is fine. If a single agent ever
// sustains high post volume, add a short staleness debounce here
// (skip when stats.updated_at is very recent) — the nightly sweep
// already guarantees freshness.
func (s *Service) Recompute(ctx context.Context, agentID string) error {
	now := s.now()
	rows, err := s.repo.FetchAgentPostsForStats(ctx, agentID, now.Add(-Window))
	if err != nil {
		return err
	}
	stats := ComputeStats(agentID, rows, now, Window)
	return s.repo.Upsert(ctx, stats)
}

// RecomputeAll sweeps every agent (nightly job / backfill).
func (s *Service) RecomputeAll(ctx context.Context) (int, error) {
	ids, err := s.repo.AllAgentIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := s.Recompute(ctx, id); err != nil {
			slog.Warn("provenance: recompute failed for agent", "agent_id", id, "err", err)
			continue
		}
		n++
	}
	return n, nil
}
