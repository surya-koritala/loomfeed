package handlers_test

import (
	"context"
	"errors"
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

type searchTestEmbedder struct {
	vector []float32
	err    error
	calls  []string
}

func (e *searchTestEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls = append(e.calls, text)
	return e.vector, e.err
}

func (e *searchTestEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, errors.New("unexpected EmbedBatch call")
}

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

func TestSearchHandler_HybridMode_CursorIsOpaqueAndStableAcrossInsert(t *testing.T) {
	handler, participants, communities, postRepo, cfg := setupSearchTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "search-cursor@example.com", "Search Cursor")
	community := createTestCommunity(t, communities, participant.ID, "search-cursor-test")
	create := func(title string) {
		t.Helper()
		if _, err := postRepo.Create(context.Background(), &models.Post{
			CommunityID: community.ID,
			AuthorID:    participant.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       title,
			Body:        "cursor pagination search body",
		}); err != nil {
			t.Fatalf("create searchable post: %v", err)
		}
	}
	for _, title := range []string{"Cursor pagination alpha", "Cursor pagination beta", "Cursor pagination gamma", "Cursor pagination delta"} {
		create(title)
	}

	firstRec := httptest.NewRecorder()
	handler.Search(firstRec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=cursor+pagination&limit=2", nil))
	testutil.AssertStatus(t, firstRec, http.StatusOK)
	var first struct {
		Data       []models.SearchResult `json:"data"`
		NextCursor string                `json:"next_cursor"`
	}
	testutil.DecodeResponse(t, firstRec, &first)
	if len(first.Data) != 2 {
		t.Fatalf("expected two search results on first page, got %d", len(first.Data))
	}
	_, cursorID, ok := handlers.DecodeCursor(first.NextCursor)
	if !ok || cursorID != first.Data[len(first.Data)-1].ID {
		t.Fatalf("expected opaque search cursor for %s, got %q", first.Data[len(first.Data)-1].ID, first.NextCursor)
	}

	create("Cursor pagination newest")
	secondRec := httptest.NewRecorder()
	handler.Search(secondRec, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=cursor+pagination&limit=2&cursor="+first.NextCursor, nil))
	testutil.AssertStatus(t, secondRec, http.StatusOK)
	var second struct {
		Data []models.SearchResult `json:"data"`
	}
	testutil.DecodeResponse(t, secondRec, &second)
	seen := map[string]bool{}
	for _, result := range first.Data {
		seen[result.ID] = true
	}
	for _, result := range second.Data {
		if seen[result.ID] {
			t.Fatalf("cursor page repeated search result %s after concurrent insert", result.ID)
		}
	}
}

func TestSearchHandler_HybridMode_UsesQueryEmbedding(t *testing.T) {
	handler, participants, communities, postRepo, cfg := setupSearchTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "search-semantic@example.com", "Search Semantic")
	community := createTestCommunity(t, communities, participant.ID, "search-semantic-test")

	post, err := postRepo.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Caring for tomato seedlings",
		Body:        "Keep young plants warm and water the soil gently.",
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}
	vector := make([]float32, 3072)
	vector[0] = 1
	if err := postRepo.SetEmbedding(context.Background(), post.ID, vector); err != nil {
		t.Fatalf("setting post embedding: %v", err)
	}
	embedder := &searchTestEmbedder{vector: vector}
	handler.WithEmbedder(embedder)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=best+way+to+nurture+juvenile+garden+plants", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp models.SearchResponse
	testutil.DecodeResponse(t, rec, &resp)
	if len(embedder.calls) != 1 || embedder.calls[0] != "best way to nurture juvenile garden plants" {
		t.Fatalf("expected one query embedding call, got %#v", embedder.calls)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || resp.Data[0].ID != post.ID {
		t.Fatalf("expected semantic-only post %s, total=%d data=%#v", post.ID, resp.Total, resp.Data)
	}
}

func TestSearchHandler_HybridMode_EmbeddingFailureFallsBackToLexical(t *testing.T) {
	handler, participants, communities, postRepo, cfg := setupSearchTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "search-fallback@example.com", "Search Fallback")
	community := createTestCommunity(t, communities, participant.ID, "search-fallback-test")

	post, err := postRepo.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Resilient lexical search",
		Body:        "Search remains available when the embedding provider is unavailable.",
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}
	embedder := &searchTestEmbedder{err: errors.New("provider unavailable")}
	handler.WithEmbedder(embedder)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=resilient+lexical", nil)
	rec := httptest.NewRecorder()
	handler.Search(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp models.SearchResponse
	testutil.DecodeResponse(t, rec, &resp)
	if len(resp.Data) == 0 || resp.Data[0].ID != post.ID {
		t.Fatalf("expected lexical fallback post %s, got %#v", post.ID, resp.Data)
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
