package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestAgentCapabilityRepo_RegisterAndList(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-1")

	inputSchema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)
	outputSchema := json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}}}`)

	// Register a capability
	cap, err := capRepo.Register(ctx, owner.ID, "research", "Deep research on topics", inputSchema, outputSchema, "https://example.com/research")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if cap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if cap.AgentID != owner.ID {
		t.Errorf("expected agent_id %q, got %q", owner.ID, cap.AgentID)
	}
	if cap.Capability != "research" {
		t.Errorf("expected capability 'research', got %q", cap.Capability)
	}
	if cap.Description != "Deep research on topics" {
		t.Errorf("expected description 'Deep research on topics', got %q", cap.Description)
	}
	if cap.EndpointURL != "https://example.com/research" {
		t.Errorf("expected endpoint_url 'https://example.com/research', got %q", cap.EndpointURL)
	}
	if cap.IsVerified {
		t.Error("expected is_verified to be false")
	}
	if cap.UsageCount != 0 {
		t.Errorf("expected usage_count 0, got %d", cap.UsageCount)
	}

	// Register another capability
	_, err = capRepo.Register(ctx, owner.ID, "summarization", "Summarize articles", nil, nil, "")
	if err != nil {
		t.Fatalf("Register second capability: %v", err)
	}

	// List capabilities
	caps, err := capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}
}

func TestAgentCapabilityRepo_Upsert(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-upsert")

	// Register
	cap1, err := capRepo.Register(ctx, owner.ID, "research", "Old description", nil, nil, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Upsert with new description
	cap2, err := capRepo.Register(ctx, owner.ID, "research", "Updated description", nil, nil, "https://new-url.com")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Should have the same ID
	if cap1.ID != cap2.ID {
		t.Errorf("expected same ID on upsert, got %q and %q", cap1.ID, cap2.ID)
	}
	if cap2.Description != "Updated description" {
		t.Errorf("expected updated description, got %q", cap2.Description)
	}
	if cap2.EndpointURL != "https://new-url.com" {
		t.Errorf("expected updated endpoint_url, got %q", cap2.EndpointURL)
	}

	// Should still be only 1 capability
	caps, err := capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	if len(caps) != 1 {
		t.Errorf("expected 1 capability after upsert, got %d", len(caps))
	}
}

func TestAgentCapabilityRepo_Unregister(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-unreg")

	_, err := capRepo.Register(ctx, owner.ID, "research", "Test", nil, nil, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Unregister
	if err := capRepo.Unregister(ctx, owner.ID, "research"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	// Should be gone
	caps, err := capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("expected 0 capabilities after unregister, got %d", len(caps))
	}
}

func TestAgentCapabilityRepo_UnregisterNotFound(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-unreg-nf")

	err := capRepo.Unregister(ctx, owner.ID, "nonexistent")
	if err == nil {
		t.Error("expected error for unregistering nonexistent capability")
	}
}

func TestAgentCapabilityRepo_Search(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner1 := createTestOwner(t, pRepo, ctx, "cap-search-1")
	owner2 := createTestOwner(t, pRepo, ctx, "cap-search-2")

	// Register capabilities for two agents
	_, err := capRepo.Register(ctx, owner1.ID, "research", "Agent 1 research", nil, nil, "")
	if err != nil {
		t.Fatalf("Register agent 1: %v", err)
	}
	_, err = capRepo.Register(ctx, owner2.ID, "research", "Agent 2 research", nil, nil, "")
	if err != nil {
		t.Fatalf("Register agent 2: %v", err)
	}
	_, err = capRepo.Register(ctx, owner1.ID, "translation", "Agent 1 translation", nil, nil, "")
	if err != nil {
		t.Fatalf("Register agent 1 translation: %v", err)
	}

	// Search for "research" capability
	results, total, err := capRepo.Search(ctx, "research", 0, false, 20, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Search for "translation" capability
	results, total, err = capRepo.Search(ctx, "translation", 0, false, 20, 0)
	if err != nil {
		t.Fatalf("Search translation: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 for translation, got %d", total)
	}

	// Search all (empty capability)
	results, total, err = capRepo.Search(ctx, "", 0, false, 20, 0)
	if err != nil {
		t.Fatalf("Search all: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3 for all, got %d", total)
	}
	_ = results
}

func TestAgentCapabilityRepo_IncrementUsage(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-usage")

	cap, err := capRepo.Register(ctx, owner.ID, "research", "Test", nil, nil, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Increment usage
	if err := capRepo.IncrementUsage(ctx, cap.ID); err != nil {
		t.Fatalf("IncrementUsage: %v", err)
	}

	// Verify count
	caps, err := capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	if len(caps) != 1 {
		t.Fatal("expected 1 capability")
	}
	if caps[0].UsageCount != 1 {
		t.Errorf("expected usage_count 1, got %d", caps[0].UsageCount)
	}
}

func TestAgentCapabilityRepo_Rate(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-rate")

	cap, err := capRepo.Register(ctx, owner.ID, "research", "Test", nil, nil, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Rate capability
	if err := capRepo.Rate(ctx, cap.ID, 4.0); err != nil {
		t.Fatalf("Rate: %v", err)
	}

	// Check result
	caps, err := capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	if len(caps) != 1 {
		t.Fatal("expected 1 capability")
	}
	if caps[0].UsageCount != 1 {
		t.Errorf("expected usage_count 1 after rate, got %d", caps[0].UsageCount)
	}
	if caps[0].AvgRating != 4.0 {
		t.Errorf("expected avg_rating 4.0, got %f", caps[0].AvgRating)
	}

	// Rate again
	if err := capRepo.Rate(ctx, cap.ID, 2.0); err != nil {
		t.Fatalf("Rate again: %v", err)
	}

	caps, err = capRepo.GetByAgent(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetByAgent: %v", err)
	}
	// Average of 4.0 and 2.0 = 3.0
	if caps[0].AvgRating != 3.0 {
		t.Errorf("expected avg_rating 3.0, got %f", caps[0].AvgRating)
	}
}

func TestAgentCapabilityRepo_RateInvalidRange(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-rate-inv")

	cap, err := capRepo.Register(ctx, owner.ID, "research", "Test", nil, nil, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := capRepo.Rate(ctx, cap.ID, 6.0); err == nil {
		t.Error("expected error for rating > 5")
	}
	if err := capRepo.Rate(ctx, cap.ID, -1.0); err == nil {
		t.Error("expected error for negative rating")
	}
}

func TestAgentCapabilityRepo_MaxLimit(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_capabilities", "agent_subscriptions", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	capRepo := repository.NewAgentCapabilityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "cap-limit")

	// Register 20 capabilities (the max)
	for i := 0; i < 20; i++ {
		_, err := capRepo.Register(ctx, owner.ID, fmt.Sprintf("cap-%d", i), "Test", nil, nil, "")
		if err != nil {
			t.Fatalf("Register cap %d: %v", i, err)
		}
	}

	// The 21st should fail
	_, err := capRepo.Register(ctx, owner.ID, "one-too-many", "Test", nil, nil, "")
	if err == nil {
		t.Error("expected error for 21st capability, got nil")
	}
}
