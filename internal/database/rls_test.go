package database_test

import (
	"context"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
)

func TestWithUserID_SetsAndGets(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "user-123")
	got := database.UserIDFromContext(ctx)
	if got != "user-123" {
		t.Errorf("expected 'user-123', got %q", got)
	}
}

func TestUserIDFromContext_EmptyWhenNotSet(t *testing.T) {
	got := database.UserIDFromContext(context.Background())
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
