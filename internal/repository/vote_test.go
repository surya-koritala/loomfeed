package repository_test

import (
	"context"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestVoteRepo_Upvote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "votes", "comments", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	voteRepo := repository.NewVoteRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "vote-upvote")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "vote-upvote")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Upvote Post")

	score, err := voteRepo.CastVote(ctx, &models.Vote{
		TargetID:   post.ID,
		TargetType: models.TargetPost,
		VoterID:    owner.ID,
		VoterType:  models.ParticipantHuman,
		Direction:  models.VoteUp,
	})
	if err != nil {
		t.Fatalf("CastVote upvote: %v", err)
	}

	if score != 1 {
		t.Errorf("expected score 1 after upvote, got %d", score)
	}

	// Verify score persisted in the posts table
	updated, err := postRepo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID post: %v", err)
	}
	if updated.VoteScore != 1 {
		t.Errorf("expected post.vote_score 1, got %d", updated.VoteScore)
	}
}

func TestVoteRepo_ToggleOff(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "votes", "comments", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	voteRepo := repository.NewVoteRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "vote-toggle")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "vote-toggle")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Toggle Vote Post")

	v := &models.Vote{
		TargetID:   post.ID,
		TargetType: models.TargetPost,
		VoterID:    owner.ID,
		VoterType:  models.ParticipantHuman,
		Direction:  models.VoteUp,
	}

	// First upvote
	score, err := voteRepo.CastVote(ctx, v)
	if err != nil {
		t.Fatalf("CastVote first upvote: %v", err)
	}
	if score != 1 {
		t.Errorf("expected score 1 after first upvote, got %d", score)
	}

	// Second upvote (same direction) → toggle off
	score, err = voteRepo.CastVote(ctx, v)
	if err != nil {
		t.Fatalf("CastVote second upvote (toggle off): %v", err)
	}
	if score != 0 {
		t.Errorf("expected score 0 after toggle off, got %d", score)
	}

	// Verify score persisted
	updated, err := postRepo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID post: %v", err)
	}
	if updated.VoteScore != 0 {
		t.Errorf("expected post.vote_score 0 after toggle off, got %d", updated.VoteScore)
	}
}

func TestVoteRepo_ChangeDirection(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "votes", "comments", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	voteRepo := repository.NewVoteRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "vote-changedir")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "vote-changedir")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Change Direction Post")

	// Upvote first
	score, err := voteRepo.CastVote(ctx, &models.Vote{
		TargetID:   post.ID,
		TargetType: models.TargetPost,
		VoterID:    owner.ID,
		VoterType:  models.ParticipantHuman,
		Direction:  models.VoteUp,
	})
	if err != nil {
		t.Fatalf("CastVote upvote: %v", err)
	}
	if score != 1 {
		t.Errorf("expected score 1 after upvote, got %d", score)
	}

	// Then downvote (change direction)
	score, err = voteRepo.CastVote(ctx, &models.Vote{
		TargetID:   post.ID,
		TargetType: models.TargetPost,
		VoterID:    owner.ID,
		VoterType:  models.ParticipantHuman,
		Direction:  models.VoteDown,
	})
	if err != nil {
		t.Fatalf("CastVote downvote: %v", err)
	}
	if score != -1 {
		t.Errorf("expected score -1 after downvote, got %d", score)
	}

	// Verify score persisted
	updated, err := postRepo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID post: %v", err)
	}
	if updated.VoteScore != -1 {
		t.Errorf("expected post.vote_score -1 after direction change, got %d", updated.VoteScore)
	}
}
