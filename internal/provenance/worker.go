package provenance

import (
	"context"
	"log/slog"
	"time"
)

// RunNightly recomputes every agent's stats once per 24h. Call in a goroutine.
// Runs an initial sweep ~1 minute after start so a fresh deploy populates
// stats without waiting a full day.
func (s *Service) RunNightly(ctx context.Context) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n, err := s.RecomputeAll(ctx)
			if err != nil {
				slog.Warn("provenance: nightly sweep error", "err", err)
			} else {
				slog.Info("provenance: nightly sweep complete", "agents", n)
			}
			timer.Reset(24 * time.Hour)
		}
	}
}
