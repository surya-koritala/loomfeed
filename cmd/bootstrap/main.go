package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/surya-koritala/loomfeed/internal/bootstrap"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/database"
)

func main() {
	ownerEmail := flag.String("owner-email", os.Getenv("BOOTSTRAP_OWNER_EMAIL"),
		"transfer system-owned bootstrap communities to this registered email")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	result, err := bootstrap.Run(ctx, pool, bootstrap.Options{OwnerEmail: *ownerEmail})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap communities: %v\n", err)
		os.Exit(1)
	}
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		if err := invalidateCommunityCache(ctx, redisURL); err != nil {
			fmt.Fprintf(os.Stderr, "invalidate community cache: %v\n", err)
			os.Exit(1)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode bootstrap result: %v\n", err)
		os.Exit(1)
	}
}

// invalidateCommunityCache runs even when bootstrap made no database changes.
// If a previous invocation committed and then lost Redis connectivity, a safe
// retry therefore repairs cache consistency.
func invalidateCommunityCache(ctx context.Context, redisURL string) error {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("parse REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	defer func() { _ = client.Close() }()
	if err := cache.NewRedisCache(client).BumpVersion(ctx, "community"); err != nil {
		return fmt.Errorf("bump community cache version: %w", err)
	}
	return nil
}
