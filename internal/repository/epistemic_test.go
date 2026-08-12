package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestEpistemicRepo_RecordsFirstContestedAtOnce(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "reputation_events", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "first-contested")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "first-contested")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "First contested timestamp")

	if err := eRepo.Vote(ctx, post.ID, owner.ID, "contested"); err != nil {
		t.Fatalf("first contested vote: %v", err)
	}

	var first time.Time
	if err := pool.QueryRow(ctx, `SELECT first_contested_at FROM posts WHERE id = $1`, post.ID).Scan(&first); err != nil {
		t.Fatalf("read first_contested_at: %v", err)
	}

	if err := eRepo.Vote(ctx, post.ID, owner.ID, "supported"); err != nil {
		t.Fatalf("move away from contested: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := eRepo.Vote(ctx, post.ID, owner.ID, "contested"); err != nil {
		t.Fatalf("second contested vote: %v", err)
	}

	var got time.Time
	if err := pool.QueryRow(ctx, `SELECT first_contested_at FROM posts WHERE id = $1`, post.ID).Scan(&got); err != nil {
		t.Fatalf("read stable first_contested_at: %v", err)
	}
	if !got.Equal(first) {
		t.Fatalf("first_contested_at changed from %s to %s", first, got)
	}
}

func TestPostRepo_RetractRecordsStableTimestamp(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "retracted-at")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "retracted-at")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Retraction timestamp")

	if err := postRepo.Retract(ctx, post.ID, "first notice"); err != nil {
		t.Fatalf("first retract: %v", err)
	}

	var first time.Time
	if err := pool.QueryRow(ctx, `SELECT retracted_at FROM posts WHERE id = $1`, post.ID).Scan(&first); err != nil {
		t.Fatalf("read retracted_at: %v", err)
	}

	time.Sleep(time.Millisecond)
	if err := postRepo.Retract(ctx, post.ID, "updated notice"); err != nil {
		t.Fatalf("second retract: %v", err)
	}

	var got time.Time
	if err := pool.QueryRow(ctx, `SELECT retracted_at FROM posts WHERE id = $1`, post.ID).Scan(&got); err != nil {
		t.Fatalf("read stable retracted_at: %v", err)
	}
	if !got.Equal(first) {
		t.Fatalf("retracted_at changed from %s to %s", first, got)
	}
}

func TestEpistemicRepo_Vote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "epistemic-vote")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "epistemic-vote")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Epistemic Vote Post")

	// Cast a vote
	err := eRepo.Vote(ctx, post.ID, owner.ID, "supported")
	if err != nil {
		t.Fatalf("Vote: %v", err)
	}

	// Check user vote
	userVote, err := eRepo.GetUserVote(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetUserVote: %v", err)
	}
	if userVote != "supported" {
		t.Errorf("expected user vote 'supported', got %q", userVote)
	}

	// Check post status
	result, err := eRepo.GetPostStatus(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPostStatus: %v", err)
	}
	if result.Status != "supported" {
		t.Errorf("expected status 'supported', got %q", result.Status)
	}
	if result.TotalVotes != 1 {
		t.Errorf("expected 1 total vote, got %d", result.TotalVotes)
	}
	if result.Counts["supported"] != 1 {
		t.Errorf("expected supported count 1, got %d", result.Counts["supported"])
	}
}

func TestEpistemicRepo_UpsertVote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "epistemic-upsert")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "epistemic-upsert")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Epistemic Upsert Post")

	// Vote supported
	if err := eRepo.Vote(ctx, post.ID, owner.ID, "supported"); err != nil {
		t.Fatalf("Vote supported: %v", err)
	}

	// Change vote to contested
	if err := eRepo.Vote(ctx, post.ID, owner.ID, "contested"); err != nil {
		t.Fatalf("Vote contested: %v", err)
	}

	// Check user vote changed
	userVote, err := eRepo.GetUserVote(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetUserVote: %v", err)
	}
	if userVote != "contested" {
		t.Errorf("expected user vote 'contested', got %q", userVote)
	}

	// Should still be 1 total vote
	result, err := eRepo.GetPostStatus(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPostStatus: %v", err)
	}
	if result.TotalVotes != 1 {
		t.Errorf("expected 1 total vote after upsert, got %d", result.TotalVotes)
	}
	if result.Status != "contested" {
		t.Errorf("expected status 'contested', got %q", result.Status)
	}
}

func TestEpistemicRepo_TiebreakPriority(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	// Create two voters
	voter1 := createTestOwner(t, pRepo, ctx, "epistemic-tie1")
	voter2 := createTestOwner(t, pRepo, ctx, "epistemic-tie2")
	community := createTestCommunity(t, cRepo, ctx, voter1.ID, "epistemic-tie")
	post := createTestPost(t, postRepo, ctx, community.ID, voter1.ID, "Epistemic Tie Post")

	// One vote for hypothesis, one for consensus -> tie at 1 each
	if err := eRepo.Vote(ctx, post.ID, voter1.ID, "hypothesis"); err != nil {
		t.Fatalf("Vote hypothesis: %v", err)
	}
	if err := eRepo.Vote(ctx, post.ID, voter2.ID, "consensus"); err != nil {
		t.Fatalf("Vote consensus: %v", err)
	}

	result, err := eRepo.GetPostStatus(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPostStatus: %v", err)
	}

	// consensus has higher priority than hypothesis in tie
	if result.Status != "consensus" {
		t.Errorf("expected 'consensus' to win tiebreak, got %q", result.Status)
	}
	if result.TotalVotes != 2 {
		t.Errorf("expected 2 total votes, got %d", result.TotalVotes)
	}
}

func TestEpistemicRepo_GetUserVote_NoVote(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "epistemic-novote")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "epistemic-novote")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Epistemic No Vote Post")

	userVote, err := eRepo.GetUserVote(ctx, post.ID, owner.ID)
	if err != nil {
		t.Fatalf("GetUserVote: %v", err)
	}
	if userVote != "" {
		t.Errorf("expected empty string for no vote, got %q", userVote)
	}
}

func TestEpistemicRepo_GetPostStatus_NonexistentPost(t *testing.T) {
	pool := database.TestPool(t)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	result, err := eRepo.GetPostStatus(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetPostStatus for nonexistent post: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nonexistent post, got %+v", result)
	}
}

func TestEpistemicRepo_MultipleVotersWinner(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "votes", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	eRepo := repository.NewEpistemicRepo(pool)
	ctx := context.Background()

	voters := make([]string, 5)
	for i := 0; i < 5; i++ {
		v := createTestOwner(t, pRepo, ctx, fmt.Sprintf("epistemic-multi-%d", i))
		voters[i] = v.ID
	}
	community := createTestCommunity(t, cRepo, ctx, voters[0], "epistemic-multi")
	post := createTestPost(t, postRepo, ctx, community.ID, voters[0], "Epistemic Multi Post")

	// 3 votes for supported, 2 for contested
	for i := 0; i < 3; i++ {
		if err := eRepo.Vote(ctx, post.ID, voters[i], "supported"); err != nil {
			t.Fatalf("Vote supported %d: %v", i, err)
		}
	}
	for i := 3; i < 5; i++ {
		if err := eRepo.Vote(ctx, post.ID, voters[i], "contested"); err != nil {
			t.Fatalf("Vote contested %d: %v", i, err)
		}
	}

	result, err := eRepo.GetPostStatus(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetPostStatus: %v", err)
	}
	if result.Status != "supported" {
		t.Errorf("expected 'supported' to win with 3 votes, got %q", result.Status)
	}
	if result.TotalVotes != 5 {
		t.Errorf("expected 5 total votes, got %d", result.TotalVotes)
	}
	if result.Counts["supported"] != 3 {
		t.Errorf("expected supported count 3, got %d", result.Counts["supported"])
	}
	if result.Counts["contested"] != 2 {
		t.Errorf("expected contested count 2, got %d", result.Counts["contested"])
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []string{"hypothesis", "supported", "contested", "refuted", "consensus"}
	for _, s := range valid {
		if !repository.IsValidStatus(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []string{"unknown", "maybe", "", "HYPOTHESIS", "Supported"}
	for _, s := range invalid {
		if repository.IsValidStatus(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}
