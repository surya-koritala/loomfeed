package repository_test

import (
	"context"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func TestPostClaimRepo_ReplaceAllAndList(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"claim_citations", "post_claims",
		"posts", "community_subscriptions", "communities",
		"agent_identities", "human_users", "participants",
	)

	ctx := context.Background()
	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	claimRepo := repository.NewPostClaimRepo(pool)

	owner := createTestOwner(t, pRepo, ctx, "post-claim")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "post-claim")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Claim Post")

	title1 := "Reuters — Q4 revenue surged 38%"
	quote1 := "Revenue climbed 38% year-over-year."
	conf := 0.92
	inputs := []repository.PostClaimInput{
		{
			ClaimText: "Revenue grew 38% last quarter.",
			Citations: []repository.ClaimCitationInput{
				{
					SourceURL:   "https://reuters.com/q4",
					SourceTitle: &title1,
					Quote:       &quote1,
					Relation:    "supports",
					Confidence:  &conf,
				},
			},
		},
		{
			ClaimText: "Margins remained flat.",
			Citations: []repository.ClaimCitationInput{
				{SourceURL: "https://sec.gov/filing", Relation: "extends"},
			},
		},
	}

	saved, err := claimRepo.ReplaceAll(ctx, post.ID, inputs)
	if err != nil {
		t.Fatalf("ReplaceAll initial: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved claims, got %d", len(saved))
	}
	if len(saved[0].Citations) != 1 || saved[0].Citations[0].Relation != "supports" {
		t.Errorf("expected first claim to have one supports citation, got %+v", saved[0].Citations)
	}

	listed, err := claimRepo.ListByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListByPost: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 listed claims, got %d", len(listed))
	}
	if listed[0].Position > listed[1].Position {
		t.Errorf("expected claims ordered by position, got %d then %d", listed[0].Position, listed[1].Position)
	}
	if listed[1].Citations[0].Relation != "extends" {
		t.Errorf("expected second claim relation 'extends', got %q", listed[1].Citations[0].Relation)
	}

	// Replace with a smaller set — old claims should be soft-deleted and hidden from List.
	updated, err := claimRepo.ReplaceAll(ctx, post.ID, []repository.PostClaimInput{
		{ClaimText: "Rewritten single claim.", Citations: nil},
	})
	if err != nil {
		t.Fatalf("ReplaceAll second: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 updated claim, got %d", len(updated))
	}

	listed2, err := claimRepo.ListByPost(ctx, post.ID)
	if err != nil {
		t.Fatalf("ListByPost after replace: %v", err)
	}
	if len(listed2) != 1 {
		t.Fatalf("expected 1 claim after replace, got %d", len(listed2))
	}
	if listed2[0].ClaimText != "Rewritten single claim." {
		t.Errorf("expected rewritten text, got %q", listed2[0].ClaimText)
	}
	if len(listed2[0].Citations) != 0 {
		t.Errorf("expected no citations after rewrite, got %d", len(listed2[0].Citations))
	}
}
