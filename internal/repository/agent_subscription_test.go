package repository_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func TestAgentSubscriptionRepo_CreateAndList(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-1")

	// Create a subscription
	sub, err := subRepo.Create(ctx, owner.ID, "community", "osai", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sub.ID == "" {
		t.Error("expected non-empty ID")
	}
	if sub.AgentID != owner.ID {
		t.Errorf("expected agent_id %q, got %q", owner.ID, sub.AgentID)
	}
	if sub.SubscriptionType != "community" {
		t.Errorf("expected subscription_type 'community', got %q", sub.SubscriptionType)
	}
	if sub.FilterValue != "osai" {
		t.Errorf("expected filter_value 'osai', got %q", sub.FilterValue)
	}
	if !sub.IsActive {
		t.Error("expected is_active to be true")
	}

	// Create another subscription with webhook URL
	webhookURL := "https://example.com/hook"
	sub2, err := subRepo.Create(ctx, owner.ID, "keyword", "golang", &webhookURL)
	if err != nil {
		t.Fatalf("Create with webhook: %v", err)
	}
	if sub2.WebhookURL == nil || *sub2.WebhookURL != webhookURL {
		t.Errorf("expected webhook_url %q, got %v", webhookURL, sub2.WebhookURL)
	}

	// List subscriptions
	subs, err := subRepo.ListByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}
}

func TestAgentSubscriptionRepo_Delete(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-del")

	sub, err := subRepo.Create(ctx, owner.ID, "community", "test-community", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete should succeed for the owner
	if err := subRepo.Delete(ctx, sub.ID, owner.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// After delete, list should be empty
	subs, err := subRepo.ListByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByAgent after delete: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions after delete, got %d", len(subs))
	}
}

func TestAgentSubscriptionRepo_DeleteWrongOwner(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-own1")
	other := createTestOwner(t, pRepo, ctx, "agsub-own2")

	sub, err := subRepo.Create(ctx, owner.ID, "community", "test-community", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete by a different owner should fail
	if err := subRepo.Delete(ctx, sub.ID, other.ID); err == nil {
		t.Error("expected error when deleting with wrong owner")
	}
}

func TestAgentSubscriptionRepo_FindMatching(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-match")

	// Create community and keyword subscriptions
	_, err := subRepo.Create(ctx, owner.ID, "community", "osai", nil)
	if err != nil {
		t.Fatalf("Create community sub: %v", err)
	}
	_, err = subRepo.Create(ctx, owner.ID, "community", "general", nil)
	if err != nil {
		t.Fatalf("Create community sub 2: %v", err)
	}

	// FindMatching for community 'osai'
	matches, err := subRepo.FindMatching(ctx, "community", "osai")
	if err != nil {
		t.Fatalf("FindMatching: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected 1 match for 'osai', got %d", len(matches))
	}

	// FindMatching is case-insensitive
	matches, err = subRepo.FindMatching(ctx, "community", "OSAI")
	if err != nil {
		t.Fatalf("FindMatching case-insensitive: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected 1 match for 'OSAI', got %d", len(matches))
	}
}

func TestAgentSubscriptionRepo_FindKeywordMatches(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-kw")

	webhookURL := "https://example.com/hook"
	_, err := subRepo.Create(ctx, owner.ID, "keyword", "golang", &webhookURL)
	if err != nil {
		t.Fatalf("Create keyword sub: %v", err)
	}
	_, err = subRepo.Create(ctx, owner.ID, "keyword", "rust", &webhookURL)
	if err != nil {
		t.Fatalf("Create keyword sub 2: %v", err)
	}

	// Should match "golang"
	matches, err := subRepo.FindKeywordMatches(ctx, "I love programming in Golang and Python")
	if err != nil {
		t.Fatalf("FindKeywordMatches: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected 1 keyword match, got %d", len(matches))
	}
	if len(matches) > 0 && matches[0].FilterValue != "golang" {
		t.Errorf("expected filter_value 'golang', got %q", matches[0].FilterValue)
	}

	// Should match both
	matches, err = subRepo.FindKeywordMatches(ctx, "Comparing Golang and Rust for systems programming")
	if err != nil {
		t.Fatalf("FindKeywordMatches both: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 keyword matches, got %d", len(matches))
	}

	// No match
	matches, err = subRepo.FindKeywordMatches(ctx, "I love programming in Python")
	if err != nil {
		t.Fatalf("FindKeywordMatches none: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 keyword matches, got %d", len(matches))
	}
}

func TestAgentSubscriptionRepo_MaxLimit(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	subRepo := repository.NewAgentSubscriptionRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "agsub-limit")

	// Create 50 subscriptions (the max)
	for i := 0; i < 50; i++ {
		_, err := subRepo.Create(ctx, owner.ID, "keyword", fmt.Sprintf("keyword-%d", i), nil)
		if err != nil {
			t.Fatalf("Create sub %d: %v", i, err)
		}
	}

	// The 51st should fail
	_, err := subRepo.Create(ctx, owner.ID, "keyword", "one-too-many", nil)
	if err == nil {
		t.Error("expected error for 51st subscription, got nil")
	}
}
