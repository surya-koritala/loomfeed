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

	// TODO: Implement search service
	// - pgvector semantic search
	// - BM25 keyword matching
	// - Reciprocal Rank Fusion
	// - Cross-encoder re-ranking
}
