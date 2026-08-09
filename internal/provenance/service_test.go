package provenance

import (
	"context"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/models"
)

type fakeRepo struct {
	rows     []models.PostStatsRow
	upserted *models.AgentProvenanceStats
	agentIDs []string
}

func (f *fakeRepo) FetchAgentPostsForStats(_ context.Context, _ string, _ time.Time) ([]models.PostStatsRow, error) {
	return f.rows, nil
}
func (f *fakeRepo) Upsert(_ context.Context, s models.AgentProvenanceStats) error { f.upserted = &s; return nil }
func (f *fakeRepo) AllAgentIDs(_ context.Context) ([]string, error)               { return f.agentIDs, nil }

func TestService_Recompute(t *testing.T) {
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	f := &fakeRepo{
		rows: []models.PostStatsRow{
			{CommunityID: "bio", CreatedAt: now.AddDate(0, 0, -1), TotalSources: 2, VerifiedSources: 2},
		},
	}
	svc := &Service{repo: f, now: func() time.Time { return now }}
	if err := svc.Recompute(context.Background(), "agent-1"); err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if f.upserted == nil || f.upserted.AgentID != "agent-1" || f.upserted.PostsCounted != 1 {
		t.Fatalf("unexpected upsert: %+v", f.upserted)
	}
	if f.upserted.AvgSourcesPerPost != 2.0 {
		t.Fatalf("want avg_sources_per_post 2.0, got %v", f.upserted.AvgSourcesPerPost)
	}
	if f.upserted.PrimarySourcePct != 1.0 {
		t.Fatalf("want primary_source_pct 1.0, got %v", f.upserted.PrimarySourcePct)
	}
}
