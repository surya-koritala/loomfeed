package provenance

import (
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/models"
)

func mkRow(total, verified int, comm string, daysAgo int, now time.Time) models.PostStatsRow {
	return models.PostStatsRow{
		CommunityID:     comm,
		CreatedAt:       now.AddDate(0, 0, -daysAgo),
		TotalSources:    total,
		VerifiedSources: verified,
	}
}

func TestComputeStats_BasicMetrics(t *testing.T) {
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	rows := []models.PostStatsRow{
		mkRow(2, 2, "bio", 1, now),
		mkRow(2, 1, "bio", 8, now),
		mkRow(1, 0, "markets", 15, now),
	}
	s := ComputeStats("agent-1", rows, now, 90*24*time.Hour)

	if s.PostsCounted != 3 {
		t.Fatalf("posts_counted: want 3, got %d", s.PostsCounted)
	}
	// 5 total sources over 3 posts.
	if got := round2(s.AvgSourcesPerPost); got != 1.67 {
		t.Errorf("avg_sources_per_post: want 1.67, got %v", got)
	}
	// 3 verified of 5 total.
	if got := round2(s.PrimarySourcePct); got != 0.6 {
		t.Errorf("primary_source_pct (verified ratio): want 0.60, got %v", got)
	}
	// beat: 2 of 3 posts in "bio".
	if got := round2(s.BeatConsistencyPct); got != 0.67 {
		t.Errorf("beat_consistency_pct: want 0.67, got %v", got)
	}
}

func TestComputeStats_NoSourcesNoPanic(t *testing.T) {
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	rows := []models.PostStatsRow{mkRow(0, 0, "bio", 1, now), mkRow(0, 0, "bio", 2, now)}
	s := ComputeStats("a", rows, now, 90*24*time.Hour)
	if s.AvgSourcesPerPost != 0 || s.PrimarySourcePct != 0 {
		t.Fatalf("zero sources should yield zero source metrics, got %+v", s)
	}
	if s.BeatConsistencyPct != 1.0 {
		t.Fatalf("all posts in one beat => 1.0, got %v", s.BeatConsistencyPct)
	}
}

func TestComputeStats_EmptyRows(t *testing.T) {
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	s := ComputeStats("a", nil, now, 90*24*time.Hour)
	if s.PostsCounted != 0 {
		t.Fatalf("want 0 posts, got %d", s.PostsCounted)
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
