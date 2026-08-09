package repository_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func setupHybridSearchTest(t *testing.T) (*repository.HybridSearchRepo, *repository.PostRepo, *repository.ParticipantRepo, *repository.CommunityRepo) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"provenances", "votes", "comments", "posts",
		"community_subscriptions", "communities",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	return repository.NewHybridSearchRepo(pool),
		repository.NewPostRepo(pool),
		repository.NewParticipantRepo(pool),
		repository.NewCommunityRepo(pool)
}

func TestHybridSearch_BasicTextMatch(t *testing.T) {
	hybridSearch, postRepo, partRepo, commRepo := setupHybridSearchTest(t)
	ctx := context.Background()

	owner := createTestOwner(t, partRepo, ctx, fmt.Sprintf("hyb-text-%d", 1))
	community := createTestCommunity(t, commRepo, ctx, owner.ID, "hyb-text-1")

	// Create posts with different titles and bodies
	_, err := postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Understanding MCP Protocol Design",
		Body:        "The Model Context Protocol is a way for AI agents to communicate with tools and services.",
	})
	if err != nil {
		t.Fatalf("creating post 1: %v", err)
	}

	_, err = postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Introduction to Quantum Computing",
		Body:        "Quantum computers use qubits to perform computations in parallel.",
	})
	if err != nil {
		t.Fatalf("creating post 2: %v", err)
	}

	// Search for "MCP protocol" -- should find the first post
	results, total, err := hybridSearch.HybridSearch(ctx, "MCP protocol", 25, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if total == 0 {
		t.Fatal("expected at least 1 result, got 0")
	}

	if results[0].Title != "Understanding MCP Protocol Design" {
		t.Errorf("expected first result to be MCP post, got %q", results[0].Title)
	}

	// Relevance score should be normalized to 1.0 for top result
	if results[0].RelevanceScore != 1.0 {
		t.Errorf("expected top result relevance_score = 1.0, got %f", results[0].RelevanceScore)
	}
}

func TestHybridSearch_EmptyResults(t *testing.T) {
	hybridSearch, _, _, _ := setupHybridSearchTest(t)
	ctx := context.Background()

	results, total, err := hybridSearch.HybridSearch(ctx, "nonexistent gibberish xyz", 25, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestHybridSearch_Pagination(t *testing.T) {
	hybridSearch, postRepo, partRepo, commRepo := setupHybridSearchTest(t)
	ctx := context.Background()

	owner := createTestOwner(t, partRepo, ctx, fmt.Sprintf("hyb-page-%d", 1))
	community := createTestCommunity(t, commRepo, ctx, owner.ID, "hyb-page-1")

	// Create 5 posts all matching "golang"
	for i := 0; i < 5; i++ {
		_, err := postRepo.Create(ctx, &models.Post{
			CommunityID: community.ID,
			AuthorID:    owner.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       fmt.Sprintf("Golang Tutorial Part %d", i+1),
			Body:        "Learn Go programming language basics and advanced topics.",
		})
		if err != nil {
			t.Fatalf("creating post %d: %v", i, err)
		}
	}

	// Page 1: limit 2, offset 0
	results1, total, err := hybridSearch.HybridSearch(ctx, "golang", 2, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch page 1: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(results1) != 2 {
		t.Errorf("expected 2 results on page 1, got %d", len(results1))
	}

	// Page 2: limit 2, offset 2
	results2, _, err := hybridSearch.HybridSearch(ctx, "golang", 2, 2, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch page 2: %v", err)
	}
	if len(results2) != 2 {
		t.Errorf("expected 2 results on page 2, got %d", len(results2))
	}

	// Results should not overlap
	for _, r1 := range results1 {
		for _, r2 := range results2 {
			if r1.ID == r2.ID {
				t.Errorf("page 1 and page 2 contain duplicate ID: %s", r1.ID)
			}
		}
	}
}

func TestHybridSearch_TitleBoost(t *testing.T) {
	hybridSearch, postRepo, partRepo, commRepo := setupHybridSearchTest(t)
	ctx := context.Background()

	owner := createTestOwner(t, partRepo, ctx, fmt.Sprintf("hyb-boost-%d", 1))
	community := createTestCommunity(t, commRepo, ctx, owner.ID, "hyb-boost-1")

	// Post A: term in title
	_, err := postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Federation Protocol Guide",
		Body:        "This guide covers how to set up servers.",
	})
	if err != nil {
		t.Fatalf("creating post A: %v", err)
	}

	// Post B: term only in body
	_, err = postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Server Configuration Tips",
		Body:        "When setting up federation protocol, make sure to configure the ports correctly.",
	})
	if err != nil {
		t.Fatalf("creating post B: %v", err)
	}

	results, _, err := hybridSearch.HybridSearch(ctx, "federation", 25, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// The post with "federation" in the title should rank higher
	if results[0].Title != "Federation Protocol Guide" {
		t.Errorf("expected title-match post to rank first, got %q", results[0].Title)
	}
}

func TestHybridSearch_RelevanceScoresNormalized(t *testing.T) {
	hybridSearch, postRepo, partRepo, commRepo := setupHybridSearchTest(t)
	ctx := context.Background()

	owner := createTestOwner(t, partRepo, ctx, fmt.Sprintf("hyb-norm-%d", 1))
	community := createTestCommunity(t, commRepo, ctx, owner.ID, "hyb-norm-1")

	for i := 0; i < 3; i++ {
		_, err := postRepo.Create(ctx, &models.Post{
			CommunityID: community.ID,
			AuthorID:    owner.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       fmt.Sprintf("Artificial Intelligence Research %d", i+1),
			Body:        "Exploring the latest developments in AI and machine learning.",
		})
		if err != nil {
			t.Fatalf("creating post %d: %v", i, err)
		}
	}

	results, _, err := hybridSearch.HybridSearch(ctx, "artificial intelligence", 25, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results, got 0")
	}

	// Top result should have relevance 1.0
	if results[0].RelevanceScore != 1.0 {
		t.Errorf("expected top result relevance 1.0, got %f", results[0].RelevanceScore)
	}

	// All scores should be between 0 and 1
	for i, r := range results {
		if r.RelevanceScore < 0 || r.RelevanceScore > 1.0 {
			t.Errorf("result %d: relevance_score %f out of range [0, 1]", i, r.RelevanceScore)
		}
	}
}

func TestHybridSearch_LimitClamping(t *testing.T) {
	hybridSearch, _, _, _ := setupHybridSearchTest(t)
	ctx := context.Background()

	// Limit should be clamped to 100 max
	_, _, err := hybridSearch.HybridSearch(ctx, "test", 200, 0, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch with large limit: %v", err)
	}

	// Negative offset should be clamped to 0
	_, _, err = hybridSearch.HybridSearch(ctx, "test", 25, -5, repository.SearchFilters{})
	if err != nil {
		t.Fatalf("HybridSearch with negative offset: %v", err)
	}
}
