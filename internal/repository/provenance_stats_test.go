package repository_test

import (
	"context"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestProvenanceStatsRepo_UpsertGet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_provenance_stats", "participants")
	pRepo := repository.NewParticipantRepo(pool)
	statsRepo := repository.NewProvenanceStatsRepo(pool)
	ctx := context.Background()

	agent := createTestOwner(t, pRepo, ctx, "prov-stats-1")

	if err := statsRepo.Upsert(ctx, models.AgentProvenanceStats{
		AgentID: agent.ID, PostsCounted: 7, AvgSourcesPerPost: 2.4, PrimarySourcePct: 0.8,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := statsRepo.Get(ctx, agent.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.PostsCounted != 7 || got.AvgSourcesPerPost != 2.4 {
		t.Fatalf("unexpected stats: %+v", got)
	}

	_ = statsRepo.Upsert(ctx, models.AgentProvenanceStats{AgentID: agent.ID, PostsCounted: 9})
	got, _ = statsRepo.Get(ctx, agent.ID)
	if got.PostsCounted != 9 {
		t.Fatalf("expected updated posts_counted 9, got %d", got.PostsCounted)
	}
}

func TestProvenanceStatsRepo_GetMissing(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_provenance_stats", "participants")
	got, err := repository.NewProvenanceStatsRepo(pool).Get(context.Background(), "00000000-0000-0000-0000-0000000000ff")
	if err != nil || got != nil {
		t.Fatalf("missing stats should be (nil,nil), got (%v,%v)", got, err)
	}
}
