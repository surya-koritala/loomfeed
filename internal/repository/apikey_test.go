package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// createTestAgent is a helper that creates an agent owned by the given human participant.
func createTestAgent(t *testing.T, pRepo *repository.ParticipantRepo, ctx context.Context, ownerID, suffix string) *models.AgentIdentity {
	t.Helper()
	agent := &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: "Agent " + suffix,
		},
		OwnerID:           ownerID,
		ModelProvider:     "openai",
		ModelName:         "gpt-4",
		MaxRPM:            60,
		ProtocolType:      models.ProtocolREST,
		HeartbeatInterval: 300,
		Capabilities:      []string{"read"},
	}
	created, err := pRepo.CreateAgent(ctx, agent)
	if err != nil {
		t.Fatalf("CreateAgent (%s): %v", suffix, err)
	}
	return created
}

// createTestAPIKey is a helper that creates an API key for a given agent.
func createTestAPIKey(t *testing.T, repo *repository.APIKeyRepo, ctx context.Context, agentID, keyHash string, scopes []string) *models.APIKey {
	t.Helper()
	k := &models.APIKey{
		AgentID:   agentID,
		KeyHash:   keyHash,
		Scopes:    scopes,
		RateLimit: 60,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		IsActive:  true,
	}
	created, err := repo.Create(ctx, k)
	if err != nil {
		t.Fatalf("Create APIKey (%s): %v", keyHash, err)
	}
	return created
}

func TestAPIKeyRepo_CreateAndGetActive(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "api_keys", "provenances", "agent_identities", "human_users", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	keyRepo := repository.NewAPIKeyRepo(pool)
	ctx := context.Background()

	// Setup: human owner -> agent -> api key
	owner := createTestOwner(t, pRepo, ctx, "apikey-create")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "apikey-create")

	created := createTestAPIKey(t, keyRepo, ctx, agent.ID, "hash_abc123", []string{"read", "write"})

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.AgentID != agent.ID {
		t.Errorf("expected agent_id %q, got %q", agent.ID, created.AgentID)
	}
	if created.KeyHash != "hash_abc123" {
		t.Errorf("expected key_hash 'hash_abc123', got %q", created.KeyHash)
	}
	if len(created.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(created.Scopes))
	}
	if created.RateLimit != 60 {
		t.Errorf("expected rate_limit 60, got %d", created.RateLimit)
	}
	if !created.IsActive {
		t.Error("expected is_active to be true")
	}

	// GetActiveByAgent
	keys, err := keyRepo.GetActiveByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetActiveByAgent: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 active key, got %d", len(keys))
	}
	if keys[0].ID != created.ID {
		t.Errorf("GetActiveByAgent returned ID %q, want %q", keys[0].ID, created.ID)
	}
}

func TestAPIKeyRepo_Revoke(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "api_keys", "provenances", "agent_identities", "human_users", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	keyRepo := repository.NewAPIKeyRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "apikey-revoke")
	agent := createTestAgent(t, pRepo, ctx, owner.ID, "apikey-revoke")
	created := createTestAPIKey(t, keyRepo, ctx, agent.ID, "hash_revoke", []string{"read"})

	// Verify key is active
	keys, err := keyRepo.GetActiveByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetActiveByAgent before revoke: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 active key before revoke, got %d", len(keys))
	}

	// Revoke the key
	if err := keyRepo.Revoke(ctx, created.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Verify key is no longer returned as active
	keys, err = keyRepo.GetActiveByAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetActiveByAgent after revoke: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 active keys after revoke, got %d", len(keys))
	}
}

func TestAPIKeyRepo_GetAllActive(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "api_keys", "provenances", "agent_identities", "human_users", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	keyRepo := repository.NewAPIKeyRepo(pool)
	ctx := context.Background()

	// Create two owners, two agents, three keys
	ownerA := createTestOwner(t, pRepo, ctx, "getallactive-a")
	ownerB := createTestOwner(t, pRepo, ctx, "getallactive-b")
	agentA := createTestAgent(t, pRepo, ctx, ownerA.ID, "getallactive-a")
	agentB := createTestAgent(t, pRepo, ctx, ownerB.ID, "getallactive-b")

	createTestAPIKey(t, keyRepo, ctx, agentA.ID, "hash_gaa_1", []string{"read"})
	createTestAPIKey(t, keyRepo, ctx, agentA.ID, "hash_gaa_2", []string{"write"})
	createTestAPIKey(t, keyRepo, ctx, agentB.ID, "hash_gab_1", []string{"read"})

	// GetAllActive should return all 3 keys
	keys, err := keyRepo.GetAllActive(ctx)
	if err != nil {
		t.Fatalf("GetAllActive: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 active keys, got %d", len(keys))
	}

	// Revoke one key and verify count drops
	if err := keyRepo.Revoke(ctx, keys[0].ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	remaining, err := keyRepo.GetAllActive(ctx)
	if err != nil {
		t.Fatalf("GetAllActive after revoke: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("expected 2 active keys after revoke, got %d", len(remaining))
	}
}
