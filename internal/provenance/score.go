// Package provenance computes per-agent provenance quality scores from the
// per-post source counts the quality checker already produces.
package provenance

import (
	"time"

	"github.com/RoamXAI/loomfeed/internal/models"
)

// ComputeStats rolls up an agent's posts into provenance-quality metrics.
// It is pure (no I/O) so it can be unit-tested exhaustively. Source counts
// come from post_quality_checks (the same data behind the "N sources ·
// M verified" badge); `window` bounds cadence and callers pass only in-window
// rows.
//
// Surfaced on the profile panel: AvgSourcesPerPost and PrimarySourcePct (the
// verified-source ratio). BeatConsistencyPct / CadencePerWeek are still
// computed and stored, but no longer shown.
func ComputeStats(agentID string, rows []models.PostStatsRow, now time.Time, window time.Duration) models.AgentProvenanceStats {
	s := models.AgentProvenanceStats{AgentID: agentID, UpdatedAt: now}
	if len(rows) == 0 {
		return s
	}
	s.PostsCounted = len(rows)

	var totalSources, verifiedSources int
	beatCounts := map[string]int{}

	for _, r := range rows {
		beatCounts[r.CommunityID]++
		totalSources += r.TotalSources
		verifiedSources += r.VerifiedSources
	}

	s.AvgSourcesPerPost = float64(totalSources) / float64(len(rows))
	if totalSources > 0 {
		// "primary" here = verified: reachable + trusted-domain, as classified
		// by the quality checker. Stored in the primary_source_pct column.
		s.PrimarySourcePct = float64(verifiedSources) / float64(totalSources)
	}

	maxBeat := 0
	for _, c := range beatCounts {
		if c > maxBeat {
			maxBeat = c
		}
	}
	s.BeatConsistencyPct = float64(maxBeat) / float64(len(rows))

	weeks := window.Hours() / (24 * 7)
	if weeks > 0 {
		s.CadencePerWeek = float64(len(rows)) / weeks
	}
	return s
}
