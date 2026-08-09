package database

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfigureRLS adds BeforeAcquire and AfterRelease hooks to the pool config
// that set/reset app.current_user_id on each connection. This enables
// PostgreSQL RLS policies to enforce per-user access control.
func ConfigureRLS(config *pgxpool.Config) {
	// Initialize session variable on new connections
	origAfterConnect := config.AfterConnect
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if origAfterConnect != nil {
			if err := origAfterConnect(ctx, conn); err != nil {
				return err
			}
		}
		_, err := conn.Exec(ctx, "SELECT set_config('app.current_user_id', '', false)")
		return err
	}

	// Set user ID when connection is acquired from pool (PrepareConn replaces deprecated BeforeAcquire)
	origPrepareConn := config.PrepareConn
	config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		if origPrepareConn != nil {
			if ok, err := origPrepareConn(ctx, conn); !ok || err != nil {
				return ok, err
			}
		}
		userID := UserIDFromContext(ctx)
		if userID != "" {
			_, err := conn.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", userID)
			if err != nil {
				slog.Error("rls: failed to set user ID on connection", "error", err)
				return false, err
			}
		}
		return true, nil
	}

	// Reset user ID when connection is released back to pool
	origAfterRelease := config.AfterRelease
	config.AfterRelease = func(conn *pgx.Conn) bool {
		if origAfterRelease != nil {
			if !origAfterRelease(conn) {
				return false
			}
		}
		_, err := conn.Exec(context.Background(), "SELECT set_config('app.current_user_id', '', false)")
		if err != nil {
			slog.Error("rls: failed to reset user ID on connection", "error", err)
			return false // discard connection
		}
		return true
	}
}
