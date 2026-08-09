package repository_test

import (
	"context"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func TestParticipantRepo_CreateHumanAndGet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "participants")

	repo := repository.NewParticipantRepo(pool)
	ctx := context.Background()

	human := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Alice",
		},
		Email:             "alice@example.com",
		PasswordHash:      "hashed_password",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}

	p, err := repo.CreateHuman(ctx, human)
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}

	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.TrustScore != 0 {
		t.Errorf("expected trust_score 0, got %f", p.TrustScore)
	}
	if p.DisplayName != "Alice" {
		t.Errorf("expected DisplayName 'Alice', got %q", p.DisplayName)
	}

	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("GetByID returned ID %q, want %q", got.ID, p.ID)
	}
	if got.DisplayName != "Alice" {
		t.Errorf("GetByID returned DisplayName %q, want 'Alice'", got.DisplayName)
	}
	if got.Type != models.ParticipantHuman {
		t.Errorf("GetByID returned Type %q, want 'human'", got.Type)
	}
}

func TestParticipantRepo_GetHumanByEmail(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "participants")

	repo := repository.NewParticipantRepo(pool)
	ctx := context.Background()

	human := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Bob",
		},
		Email:             "bob@example.com",
		PasswordHash:      "another_hash",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}

	_, err := repo.CreateHuman(ctx, human)
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}

	got, err := repo.GetHumanByEmail(ctx, "bob@example.com")
	if err != nil {
		t.Fatalf("GetHumanByEmail: %v", err)
	}
	if got.DisplayName != "Bob" {
		t.Errorf("expected DisplayName 'Bob', got %q", got.DisplayName)
	}
	if got.Email != "bob@example.com" {
		t.Errorf("expected email 'bob@example.com', got %q", got.Email)
	}
}

func TestParticipantRepo_CreateAgent(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "participants")

	repo := repository.NewParticipantRepo(pool)
	ctx := context.Background()

	// Create owner human first
	owner := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Owner Human",
		},
		Email:             "owner@example.com",
		PasswordHash:      "owner_hash",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}
	ownerParticipant, err := repo.CreateHuman(ctx, owner)
	if err != nil {
		t.Fatalf("CreateHuman (owner): %v", err)
	}

	// Create agent linked to owner
	agent := &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: "Test Agent",
		},
		OwnerID:           ownerParticipant.ID,
		ModelProvider:     "openai",
		ModelName:         "gpt-4",
		ModelVersion:      "2024-01",
		Capabilities:      []string{"reasoning", "coding"},
		MaxRPM:            60,
		ProtocolType:      models.ProtocolREST,
		HeartbeatInterval: 300,
	}

	created, err := repo.CreateAgent(ctx, agent)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if created.ID == "" {
		t.Error("expected non-empty agent ID")
	}
	if created.ModelProvider != "openai" {
		t.Errorf("expected model_provider 'openai', got %q", created.ModelProvider)
	}
	if created.OwnerID != ownerParticipant.ID {
		t.Errorf("expected owner_id %q, got %q", ownerParticipant.ID, created.OwnerID)
	}

	// Verify via GetAgentByID
	fetched, err := repo.GetAgentByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAgentByID: %v", err)
	}
	if fetched.ModelProvider != "openai" {
		t.Errorf("GetAgentByID: expected model_provider 'openai', got %q", fetched.ModelProvider)
	}
	if fetched.ModelName != "gpt-4" {
		t.Errorf("GetAgentByID: expected model_name 'gpt-4', got %q", fetched.ModelName)
	}
	if len(fetched.Capabilities) != 2 {
		t.Errorf("GetAgentByID: expected 2 capabilities, got %d", len(fetched.Capabilities))
	}
}

func TestParticipantRepo_ListAgentsByOwner(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "participants")

	repo := repository.NewParticipantRepo(pool)
	ctx := context.Background()

	// Create owner
	owner := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Agent Owner",
		},
		Email:             "agentowner@example.com",
		PasswordHash:      "pw_hash",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}
	ownerP, err := repo.CreateHuman(ctx, owner)
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}

	// Create two agents
	for i, name := range []string{"Agent One", "Agent Two"} {
		a := &models.AgentIdentity{
			Participant: models.Participant{
				DisplayName: name,
			},
			OwnerID:           ownerP.ID,
			ModelProvider:     "anthropic",
			ModelName:         "claude-3",
			MaxRPM:            60,
			ProtocolType:      models.ProtocolREST,
			HeartbeatInterval: 300,
			Capabilities:      []string{},
		}
		_ = i
		if _, err := repo.CreateAgent(ctx, a); err != nil {
			t.Fatalf("CreateAgent %q: %v", name, err)
		}
	}

	agents, err := repo.ListAgentsByOwner(ctx, ownerP.ID)
	if err != nil {
		t.Fatalf("ListAgentsByOwner: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
	for _, a := range agents {
		if a.OwnerID != ownerP.ID {
			t.Errorf("agent %q has wrong owner_id %q", a.ID, a.OwnerID)
		}
	}
}
