package repository_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// createTestOwner is a helper that creates a human participant for use as a community owner.
func createTestOwner(t *testing.T, repo *repository.ParticipantRepo, ctx context.Context, suffix string) *models.Participant {
	t.Helper()
	h := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Owner " + suffix,
		},
		Email:             fmt.Sprintf("owner-%s@example.com", suffix),
		PasswordHash:      "hashed_password",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}
	p, err := repo.CreateHuman(ctx, h)
	if err != nil {
		t.Fatalf("CreateHuman (%s): %v", suffix, err)
	}
	return p
}

func TestCommunityRepo_CreateAndGet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "community_subscriptions", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "createandget")

	community := &models.Community{
		Name:        "Test Community",
		Slug:        "test-community",
		Description: "A test community",
		Rules:       "Be kind",
		CreatedBy:   owner.ID,
	}

	created, err := cRepo.Create(ctx, community)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Slug != "test-community" {
		t.Errorf("expected slug 'test-community', got %q", created.Slug)
	}
	if created.AgentPolicy != models.AgentPolicyOpen {
		t.Errorf("expected agent_policy 'open', got %q", created.AgentPolicy)
	}
	if created.CreatedBy != owner.ID {
		t.Errorf("expected created_by %q, got %q", owner.ID, created.CreatedBy)
	}

	got, err := cRepo.GetBySlug(ctx, "test-community")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetBySlug returned ID %q, want %q", got.ID, created.ID)
	}
	if got.Name != "Test Community" {
		t.Errorf("GetBySlug returned Name %q, want 'Test Community'", got.Name)
	}
	if got.Description != "A test community" {
		t.Errorf("GetBySlug returned Description %q, want 'A test community'", got.Description)
	}
}

func TestCommunityRepo_Subscribe(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "community_subscriptions", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "subscribe")

	community := &models.Community{
		Name:      "Subscribe Community",
		Slug:      "subscribe-community",
		CreatedBy: owner.ID,
	}

	created, err := cRepo.Create(ctx, community)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	subscriber := createTestOwner(t, pRepo, ctx, "subscriber")

	if err := cRepo.Subscribe(ctx, created.ID, subscriber.ID); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	got, err := cRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SubscriberCount != 1 {
		t.Errorf("expected subscriber_count 1, got %d", got.SubscriberCount)
	}
}

func TestCommunityRepo_List(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "community_subscriptions", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "list")

	// Create two communities
	commA, err := cRepo.Create(ctx, &models.Community{
		Name:      "Community A",
		Slug:      "community-a",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}

	commB, err := cRepo.Create(ctx, &models.Community{
		Name:      "Community B",
		Slug:      "community-b",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	// Subscribe two people to A, one to B — A should rank higher
	for i, suffix := range []string{"list-sub1", "list-sub2"} {
		sub := createTestOwner(t, pRepo, ctx, suffix)
		if err := cRepo.Subscribe(ctx, commA.ID, sub.ID); err != nil {
			t.Fatalf("Subscribe A sub%d: %v", i+1, err)
		}
	}

	sub3 := createTestOwner(t, pRepo, ctx, "list-sub3")
	if err := cRepo.Subscribe(ctx, commB.ID, sub3.ID); err != nil {
		t.Fatalf("Subscribe B sub3: %v", err)
	}

	communities, err := cRepo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(communities) != 2 {
		t.Fatalf("expected 2 communities, got %d", len(communities))
	}
	// A has 2 subscribers, B has 1 — A should be first
	if communities[0].Slug != "community-a" {
		t.Errorf("expected first community slug 'community-a', got %q", communities[0].Slug)
	}
	if communities[0].SubscriberCount != 2 {
		t.Errorf("expected communities[0].subscriber_count 2, got %d", communities[0].SubscriberCount)
	}
	if communities[1].Slug != "community-b" {
		t.Errorf("expected second community slug 'community-b', got %q", communities[1].Slug)
	}
}

func TestCommunityRepo_Unsubscribe(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "community_subscriptions", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "unsub")

	created, err := cRepo.Create(ctx, &models.Community{
		Name:      "Unsub Community",
		Slug:      "unsub-community",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	subscriber := createTestOwner(t, pRepo, ctx, "unsub-member")

	if err := cRepo.Subscribe(ctx, created.ID, subscriber.ID); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Confirm subscribed
	got, err := cRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID after subscribe: %v", err)
	}
	if got.SubscriberCount != 1 {
		t.Errorf("expected subscriber_count 1 after subscribe, got %d", got.SubscriberCount)
	}

	if err := cRepo.Unsubscribe(ctx, created.ID, subscriber.ID); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	got, err = cRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID after unsubscribe: %v", err)
	}
	if got.SubscriberCount != 0 {
		t.Errorf("expected subscriber_count 0 after unsubscribe, got %d", got.SubscriberCount)
	}
}
