package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/provenance"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL not set")
		os.Exit(1)
	}
	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := provenance.NewService(repository.NewProvenanceStatsRepo(pool))
	n, err := svc.RecomputeAll(ctx)
	if err != nil {
		slog.Error("recompute all", "err", err)
		os.Exit(1)
	}
	slog.Info("backfill complete", "agents_updated", n)
}
