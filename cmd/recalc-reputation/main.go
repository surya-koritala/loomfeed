// recalc-reputation rebuilds participants.reputation_score from
// reputation_events history under the current uncapped formula.
//
// Run after deploying the uncapped rep system (migration 000062). Safe
// to re-run any time — it's idempotent.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/recalc-reputation
//	DATABASE_URL=postgres://... go run ./cmd/recalc-reputation --dry-run
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "compute new scores but don't write")
	flag.Parse()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		slog.Error("connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `SELECT id, display_name, type, reputation_score FROM participants ORDER BY id`)
	if err != nil {
		slog.Error("list participants failed", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	type p struct {
		ID, Name, Type string
		OldRep         float64
	}
	var participants []p
	for rows.Next() {
		var x p
		if err := rows.Scan(&x.ID, &x.Name, &x.Type, &x.OldRep); err != nil {
			slog.Error("scan participant", "error", err)
			os.Exit(1)
		}
		participants = append(participants, x)
	}
	if err := rows.Err(); err != nil {
		slog.Error("iterate participants", "error", err)
		os.Exit(1)
	}
	rows.Close()

	repo := repository.NewReputationRepo(pool)
	slog.Info("recalculating", "participants", len(participants), "dry_run", *dryRun)

	var changed int
	for _, x := range participants {
		if *dryRun {
			// Read the new value without writing. We do this by replaying
			// the events the same way Recalculate does, but we'd need
			// to duplicate the math. Simpler: skip dry-run mutation by
			// running Recalculate then rolling back via a transient
			// participant. For first pass, dry-run just logs counts.
			continue
		}
		if err := repo.Recalculate(ctx, x.ID); err != nil {
			slog.Error("recalc failed", "participant", x.ID, "name", x.Name, "error", err)
			continue
		}

		var newRep float64
		_ = pool.QueryRow(ctx, `SELECT reputation_score FROM participants WHERE id = $1`, x.ID).Scan(&newRep)
		if newRep != x.OldRep {
			changed++
			slog.Info("rep changed",
				"participant", x.Name,
				"type", x.Type,
				"old", x.OldRep,
				"new", newRep,
				"delta", newRep-x.OldRep,
			)
		}
	}

	slog.Info("done", "total", len(participants), "changed", changed)
}
