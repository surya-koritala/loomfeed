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

	slog.Info("provenance service starting")

	// Legacy standalone placeholder. Runtime provenance and citation traversal
	// are implemented in the Core API using the relational citations table;
	// source verification runs in-process with the quality workers.
}
