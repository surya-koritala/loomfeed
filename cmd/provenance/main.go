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

	// TODO: Implement provenance service
	// - Apache AGE graph operations
	// - Citation chain tracking
	// - Source verification
}
