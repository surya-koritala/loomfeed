package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupSearchTest(t *testing.T) (*handlers.SearchHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"provenances", "votes", "comments", "posts",
		"community_subscriptions", "communities",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	search := repository.NewSearchRepo(pool)
	hybridSearch := repository.NewHybridSearchRepo(pool)
	posts := repository.NewPostRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewSearchHandler(search, hybridSearch), participants, communities, posts, cfg
}

func TestSearchHandler_HybridMode_Default(t *testing.T) {
	handler, participants, communities, postRepo, cfg := setupSearchTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "search-hybrid@example.com", "Search Hybrid")
	community := createTestCommunity(t, communities, participant.ID, "search-hybrid-test")

	_, err := postRepo.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Testing Hybrid Search Feature",
		Body:        "This post is about testing the hybrid search functionality.",
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}

	// Default mode should be hybrid
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=hybrid+search", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp models.SearchResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Mode != "hybrid" {
		t.Errorf("expected mode 'hybrid', got %q", resp.Mode)
	}
	if resp.Query != "hybrid search" {
		t.Errorf("expected query 'hybrid search', got %q", resp.Query)
	}
	if resp.Total == 0 {
		t.Error("expected at least 1 result")
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected data to contain results")
	}
	if resp.Data[0].RelevanceScore <= 0 {
		t.Errorf("expected relevance_score > 0, got %f", resp.Data[0].RelevanceScore)
	}
}

func TestSearchHandler_TextMode_Legacy(t *testing.T) {
	handler, participants, communities, postRepo, cfg := setupSearchTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "search-text@example.com", "Search Text")
	community := createTestCommunity(t, communities, participant.ID, "search-text-test")

	_, err := postRepo.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Legacy Text Search Post",
		Body:        "This post tests the legacy full-text search mode.",
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=legacy+text&mode=text", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	// Legacy mode returns PaginatedResponse (no mode/query fields)
	var resp models.PaginatedResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Total == 0 {
		t.Error("expected at least 1 result in text mode")
	}
}

func TestSearchHandler_MissingQuery(t *testing.T) {
	handler, _, _, _, _ := setupSearchTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSearchHandler_InvalidMode(t *testing.T) {
	handler, _, _, _, _ := setupSearchTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&mode=invalid", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSearchHandler_EmptyResults(t *testing.T) {
	handler, _, _, _, _ := setupSearchTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=zxcvbnmasdfghjkl", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp models.SearchResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Data))
	}
}
