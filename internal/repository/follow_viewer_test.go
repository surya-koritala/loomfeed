package repository_test

import (
	"context"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func TestFollowedSet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"follows", "api_keys", "provenances", "agent_identities", "human_users", "posts", "communities", "participants",
	)
	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	fRepo := repository.NewFollowRepo(pool)

	human, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "F"},
		Email:             "f@example.com",
		PasswordHash:      "x",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman: %v", err)
	}
	mkAgent := func(name string) string {
		a, err := pRepo.CreateAgent(ctx, &models.AgentIdentity{
			Participant:       models.Participant{DisplayName: name},
			OwnerID:           human.ID,
			ModelProvider:     "openai",
			ModelName:         "gpt-4",
			MaxRPM:            60,
			ProtocolType:      models.ProtocolREST,
			HeartbeatInterval: 300,
			Capabilities:      []string{"read"},
		})
		if err != nil {
			t.Fatalf("CreateAgent: %v", err)
		}
		return a.ID
	}
	followed := mkAgent("Followed")
	other := mkAgent("Other")

	if err := fRepo.Follow(ctx, human.ID, followed); err != nil {
		t.Fatalf("Follow: %v", err)
	}

	set, err := fRepo.FollowedSet(ctx, human.ID, []string{followed, other})
	if err != nil {
		t.Fatalf("FollowedSet: %v", err)
	}
	if !set[followed] || set[other] {
		t.Fatalf("want {followed:true, other:false}, got %v", set)
	}

	empty, err := fRepo.FollowedSet(ctx, human.ID, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("nil ids: want empty map no error, got %v %v", empty, err)
	}

	noFollower, err := fRepo.FollowedSet(ctx, "", []string{followed})
	if err != nil || len(noFollower) != 0 {
		t.Fatalf("empty follower: want empty map no error, got %v %v", noFollower, err)
	}
}
