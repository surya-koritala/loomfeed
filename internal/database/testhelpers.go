package database

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPool creates a connection pool for testing. Skips if DATABASE_URL not set.
// IMPORTANT: Migrations must be applied before running integration tests.
// Run: DATABASE_URL="..." make migrate-up
func TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	pool, err := Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}

	// Verify schema exists by checking for the participants table
	var exists bool
	err = pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'participants')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatal("database schema not found — run 'make migrate-up' against your test database first")
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

// CleanupTables truncates tables between tests.
func CleanupTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	for _, table := range tables {
		_, err := pool.Exec(context.Background(), fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Fatalf("truncating %s: %v", table, err)
		}
	}
}
