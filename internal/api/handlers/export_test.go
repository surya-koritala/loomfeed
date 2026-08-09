package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupExportTest(t *testing.T) (*handlers.ExportHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *repository.CommentRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "provenances", "reputation_events", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewExportHandler(pool), participants, communities, posts, comments, cfg
}

func TestExportHandler_Stats_Empty(t *testing.T) {
	handler, _, _, _, _, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/stats", nil)
	rec := httptest.NewRecorder()

	handler.Stats(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if result["total_posts"].(float64) != 0 {
		t.Errorf("expected 0 total_posts, got %v", result["total_posts"])
	}
	if result["total_comments"].(float64) != 0 {
		t.Errorf("expected 0 total_comments, got %v", result["total_comments"])
	}
	if result["total_agents"].(float64) != 0 {
		t.Errorf("expected 0 total_agents, got %v", result["total_agents"])
	}
}

func TestExportHandler_Stats_WithData(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-stats@example.com", "ExportStats")
	community := createTestCommunity(t, communities, participant.ID, "export-stats-test")

	// Create some posts
	for i := 0; i < 3; i++ {
		_, err := posts.Create(context.Background(), &models.Post{
			CommunityID: community.ID,
			AuthorID:    participant.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       "Stats Test Post",
			Body:        "Test body",
			PostType:    models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("creating test post: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/stats", nil)
	rec := httptest.NewRecorder()

	handler.Stats(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if result["total_posts"].(float64) != 3 {
		t.Errorf("expected 3 total_posts, got %v", result["total_posts"])
	}

	typeDist, ok := result["post_type_distribution"].(map[string]any)
	if !ok {
		t.Fatal("expected post_type_distribution to be a map")
	}
	if typeDist["text"].(float64) != 3 {
		t.Errorf("expected 3 text posts, got %v", typeDist["text"])
	}
}

func TestExportHandler_Posts_Empty(t *testing.T) {
	handler, _, _, _, _, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var result []any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestExportHandler_Posts_WithData(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-posts@example.com", "ExportPosts")
	community := createTestCommunity(t, communities, participant.ID, "export-posts-test")

	_, err := posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Export Test Post",
		Body:        "Body for export test",
		PostType:    models.PostTypeSynthesis,
		Tags:        []string{"test", "export"},
	})
	if err != nil {
		t.Fatalf("creating test post: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 post, got %d", len(result))
	}
	if result[0]["title"] != "Export Test Post" {
		t.Errorf("expected title 'Export Test Post', got %q", result[0]["title"])
	}
	if result[0]["post_type"] != "synthesis" {
		t.Errorf("expected post_type 'synthesis', got %q", result[0]["post_type"])
	}
	if result[0]["community"] != "export-posts-test" {
		t.Errorf("expected community 'export-posts-test', got %q", result[0]["community"])
	}
	author, ok := result[0]["author"].(map[string]any)
	if !ok {
		t.Fatal("expected author to be a map")
	}
	if author["name"] != "ExportPosts" {
		t.Errorf("expected author name 'ExportPosts', got %q", author["name"])
	}
}

func TestExportHandler_Posts_JSONL(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-jsonl@example.com", "ExportJSONL")
	community := createTestCommunity(t, communities, participant.ID, "export-jsonl-test")

	for i := 0; i < 3; i++ {
		_, err := posts.Create(context.Background(), &models.Post{
			CommunityID: community.ID,
			AuthorID:    participant.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       "JSONL Test Post",
			Body:        "Body",
			PostType:    models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("creating test post: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts", nil) // default is jsonl
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("expected Content-Type application/x-ndjson, got %q", ct)
	}

	// Verify each line is valid JSON
	dec := json.NewDecoder(rec.Body)
	count := 0
	for dec.More() {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			t.Fatalf("decoding JSONL line %d: %v", count, err)
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 JSONL lines, got %d", count)
	}
}

func TestExportHandler_Posts_FilterByCommunity(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-filter@example.com", "ExportFilter")
	communityA := createTestCommunity(t, communities, participant.ID, "export-filter-a")
	communityB := createTestCommunity(t, communities, participant.ID, "export-filter-b")

	_, _ = posts.Create(context.Background(), &models.Post{
		CommunityID: communityA.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "In A",
		Body:        "body",
		PostType:    models.PostTypeText,
	})
	_, _ = posts.Create(context.Background(), &models.Post{
		CommunityID: communityB.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "In B",
		Body:        "body",
		PostType:    models.PostTypeText,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts?format=json&community=export-filter-a", nil)
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 post, got %d", len(result))
	}
	if result[0]["title"] != "In A" {
		t.Errorf("expected title 'In A', got %q", result[0]["title"])
	}
}

func TestExportHandler_Posts_FilterByPostType(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-type@example.com", "ExportType")
	community := createTestCommunity(t, communities, participant.ID, "export-type-test")

	_, _ = posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Text Post",
		Body:        "body",
		PostType:    models.PostTypeText,
	})
	_, _ = posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Debate Post",
		Body:        "body",
		PostType:    models.PostTypeDebate,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts?format=json&post_type=debate", nil)
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 post, got %d", len(result))
	}
	if result[0]["title"] != "Debate Post" {
		t.Errorf("expected title 'Debate Post', got %q", result[0]["title"])
	}
}

func TestExportHandler_Posts_Limit(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-limit@example.com", "ExportLimit")
	community := createTestCommunity(t, communities, participant.ID, "export-limit-test")

	for i := 0; i < 5; i++ {
		_, _ = posts.Create(context.Background(), &models.Post{
			CommunityID: community.ID,
			AuthorID:    participant.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       "Limit Test",
			Body:        "body",
			PostType:    models.PostTypeText,
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/posts?format=json&limit=2", nil)
	rec := httptest.NewRecorder()

	handler.Posts(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 2 {
		t.Errorf("expected 2 posts, got %d", len(result))
	}
}

func TestExportHandler_Debates_Empty(t *testing.T) {
	handler, _, _, _, _, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/debates?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Debates(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestExportHandler_Debates_WithData(t *testing.T) {
	handler, participants, communities, posts, comments, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-debate@example.com", "ExportDebate")
	community := createTestCommunity(t, communities, participant.ID, "export-debate-test")

	post, err := posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Test Debate",
		Body:        "This is the debate body",
		PostType:    models.PostTypeDebate,
	})
	if err != nil {
		t.Fatalf("creating debate post: %v", err)
	}

	// Add a comment
	_, err = comments.Create(context.Background(), &models.Comment{
		PostID:     post.ID,
		AuthorID:   participant.ID,
		AuthorType: models.ParticipantHuman,
		Body:       "I agree",
		Depth:      0,
	})
	if err != nil {
		t.Fatalf("creating comment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/debates?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Debates(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 debate, got %d", len(result))
	}
	if result[0]["title"] != "Test Debate" {
		t.Errorf("expected title 'Test Debate', got %q", result[0]["title"])
	}
	debateComments, ok := result[0]["comments"].([]any)
	if !ok {
		t.Fatal("expected comments to be an array")
	}
	if len(debateComments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(debateComments))
	}
}

func TestExportHandler_Threads_Empty(t *testing.T) {
	handler, _, _, _, _, _ := setupExportTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/threads?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Threads(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestExportHandler_Threads_WithNestedComments(t *testing.T) {
	handler, participants, communities, posts, comments, cfg := setupExportTest(t)
	participant, _ := registerTestUser(t, participants, cfg, "export-thread@example.com", "ExportThread")
	community := createTestCommunity(t, communities, participant.ID, "export-thread-test")

	post, err := posts.Create(context.Background(), &models.Post{
		CommunityID: community.ID,
		AuthorID:    participant.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Thread Test",
		Body:        "Thread body",
		PostType:    models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("creating post: %v", err)
	}

	// Create root comment
	rootComment, err := comments.Create(context.Background(), &models.Comment{
		PostID:     post.ID,
		AuthorID:   participant.ID,
		AuthorType: models.ParticipantHuman,
		Body:       "Root comment",
		Depth:      0,
	})
	if err != nil {
		t.Fatalf("creating root comment: %v", err)
	}

	// Create reply
	_, err = comments.Create(context.Background(), &models.Comment{
		PostID:          post.ID,
		ParentCommentID: &rootComment.ID,
		AuthorID:        participant.ID,
		AuthorType:      models.ParticipantHuman,
		Body:            "Reply comment",
		Depth:           1,
	})
	if err != nil {
		t.Fatalf("creating reply: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/threads?format=json", nil)
	rec := httptest.NewRecorder()

	handler.Threads(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result []map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if len(result) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(result))
	}
	if result[0]["title"] != "Thread Test" {
		t.Errorf("expected title 'Thread Test', got %q", result[0]["title"])
	}

	thread, ok := result[0]["thread"].([]any)
	if !ok {
		t.Fatal("expected thread to be an array")
	}
	if len(thread) != 1 {
		t.Fatalf("expected 1 root comment, got %d", len(thread))
	}

	rootMap, ok := thread[0].(map[string]any)
	if !ok {
		t.Fatal("expected root comment to be a map")
	}
	if rootMap["body"] != "Root comment" {
		t.Errorf("expected root body 'Root comment', got %q", rootMap["body"])
	}

	replies, ok := rootMap["replies"].([]any)
	if !ok {
		t.Fatal("expected replies to be an array")
	}
	if len(replies) != 1 {
		t.Errorf("expected 1 reply, got %d", len(replies))
	}
}
