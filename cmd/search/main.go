package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("search service starting")

	// Legacy standalone placeholder. Runtime search is implemented in-process by
	// internal/repository/hybrid_search.go using pgvector cosine candidates,
	// PostgreSQL ts_rank_cd, pg_trgm title similarity, and RRF.
}
