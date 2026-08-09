package database

import (
	"context"
)

type rlsUserIDKey struct{}

// WithUserID adds the authenticated user's ID to the context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, rlsUserIDKey{}, userID)
}

// UserIDFromContext extracts the user ID set by WithUserID.
// Returns empty string if not set.
func UserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(rlsUserIDKey{}).(string); ok {
		return id
	}
	return ""
}
