package repository_test

import (
	"context"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestVerificationRepo_VerifyAndUnverify(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "human_verifications", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	verifyRepo := repository.NewVerificationRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "verify-test")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "verify-test")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Verify Test Post")

	// Initially no verifications
	count, err := verifyRepo.GetCount(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	hasVerified, err := verifyRepo.HasVerified(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("HasVerified: %v", err)
	}
	if hasVerified {
		t.Error("expected hasVerified false, got true")
	}

	// Verify
	if err := verifyRepo.Verify(ctx, post.ID, owner.ID); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	count, err = verifyRepo.GetCount(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetCount after verify: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1 after verify, got %d", count)
	}

	hasVerified, err = verifyRepo.HasVerified(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("HasVerified after verify: %v", err)
	}
	if !hasVerified {
		t.Error("expected hasVerified true after verify, got false")
	}

	// GetVerifiers
	verifiers, err := verifyRepo.GetVerifiers(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetVerifiers: %v", err)
	}
	if len(verifiers) != 1 {
		t.Errorf("expected 1 verifier, got %d", len(verifiers))
	}
	if len(verifiers) > 0 && verifiers[0].ID != owner.ID {
		t.Errorf("expected verifier ID %s, got %s", owner.ID, verifiers[0].ID)
	}

	// Verify again (idempotent -- ON CONFLICT DO NOTHING)
	if err := verifyRepo.Verify(ctx, post.ID, owner.ID); err != nil {
		t.Fatalf("Verify again: %v", err)
	}

	count, err = verifyRepo.GetCount(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetCount after duplicate verify: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count still 1 after duplicate verify, got %d", count)
	}

	// Unverify
	if err := verifyRepo.Unverify(ctx, post.ID, owner.ID); err != nil {
		t.Fatalf("Unverify: %v", err)
	}

	count, err = verifyRepo.GetCount(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetCount after unverify: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after unverify, got %d", count)
	}

	hasVerified, err = verifyRepo.HasVerified(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("HasVerified after unverify: %v", err)
	}
	if hasVerified {
		t.Error("expected hasVerified false after unverify, got true")
	}
}

func TestVerificationRepo_MultipleVerifiers(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "human_verifications", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	verifyRepo := repository.NewVerificationRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "multi-verify-1")
	verifier2 := createTestOwner(t, pRepo, ctx, "multi-verify-2")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "multi-verify")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Multi Verify Post")

	// Two different verifiers
	if err := verifyRepo.Verify(ctx, post.ID, owner.ID); err != nil {
		t.Fatalf("Verify owner: %v", err)
	}
	if err := verifyRepo.Verify(ctx, post.ID, verifier2.ID); err != nil {
		t.Fatalf("Verify verifier2: %v", err)
	}

	count, err := verifyRepo.GetCount(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	verifiers, err := verifyRepo.GetVerifiers(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetVerifiers: %v", err)
	}
	if len(verifiers) != 2 {
		t.Errorf("expected 2 verifiers, got %d", len(verifiers))
	}
}
