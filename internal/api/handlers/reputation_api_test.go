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

func setupReputationAPITest(t *testing.T) (*handlers.ReputationAPIHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *repository.PostRepo, *repository.ReputationRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "epistemic_votes", "provenances", "reputation_events", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	reputation := repository.NewReputationRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewReputationAPIHandler(pool), participants, communities, posts, reputation, cfg
}

func TestReputationAPI_GetReputation_NotFound(t *testing.T) {
	handler, _, _, _, _, _ := setupReputationAPITest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/00000000-0000-0000-0000-000000000000", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()

	handler.GetReputation(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestReputationAPI_GetReputation_Success(t *testing.T) {
	handler, participants, communities, posts, _, cfg := setupReputationAPITest(t)
	participant, _ := registerTestUser(t, participants, cfg, "rep-api@example.com", "RepAPIUser")
	community := createTestCommunity(t, communities, participant.ID, "rep-api-test")

	// Create some posts
	for i := 0; i < 3; i++ {
		_, err := posts.Create(context.Background(), &models.Post{
			CommunityID: community.ID,
			AuthorID:    participant.ID,
			AuthorType:  models.ParticipantHuman,
			Title:       "Rep API Post",
			Body:        "Test body",
			PostType:    models.PostTypeText,
		})
		if err != nil {
			t.Fatalf("creating test post: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/"+participant.ID, nil)
	req.SetPathValue("id", participant.ID)
	rec := httptest.NewRecorder()

	handler.GetReputation(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if result["agent_id"] != participant.ID {
		t.Errorf("expected agent_id %q, got %q", participant.ID, result["agent_id"])
	}
	if result["display_name"] != "RepAPIUser" {
		t.Errorf("expected display_name 'RepAPIUser', got %q", result["display_name"])
	}
	if result["type"] != "human" {
		t.Errorf("expected type 'human', got %q", result["type"])
	}

	// Check CORS header
	if cors := rec.Header().Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("expected CORS header '*', got %q", cors)
	}

	// Check epistemic_accuracy is present
	ea, ok := result["epistemic_accuracy"].(map[string]any)
	if !ok {
		t.Fatal("expected epistemic_accuracy to be a map")
	}
	if _, ok := ea["supported_votes"]; !ok {
		t.Error("expected epistemic_accuracy to contain supported_votes")
	}

	// Check provenance_stats is present
	ps, ok := result["provenance_stats"].(map[string]any)
	if !ok {
		t.Fatal("expected provenance_stats to be a map")
	}
	if _, ok := ps["total_sourced_posts"]; !ok {
		t.Error("expected provenance_stats to contain total_sourced_posts")
	}
}

func TestReputationAPI_GetHistory_NotFound(t *testing.T) {
	handler, _, _, _, _, _ := setupReputationAPITest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/00000000-0000-0000-0000-000000000000/history", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestReputationAPI_GetHistory_Success(t *testing.T) {
	handler, participants, _, _, reputation, cfg := setupReputationAPITest(t)
	participant, _ := registerTestUser(t, participants, cfg, "rep-history@example.com", "RepHistoryUser")

	// Add some reputation events
	err := reputation.RecordEvent(context.Background(), participant.ID, "upvote_received", 0.5)
	if err != nil {
		t.Fatalf("recording reputation event: %v", err)
	}
	err = reputation.RecordEvent(context.Background(), participant.ID, "upvote_received", 0.3)
	if err != nil {
		t.Fatalf("recording reputation event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/"+participant.ID+"/history", nil)
	req.SetPathValue("id", participant.ID)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if result["agent_id"] != participant.ID {
		t.Errorf("expected agent_id %q, got %q", participant.ID, result["agent_id"])
	}

	dataPoints, ok := result["data_points"].([]any)
	if !ok {
		t.Fatal("expected data_points to be an array")
	}
	// Both events happen on the same day, so should be 1 data point
	if len(dataPoints) != 1 {
		t.Errorf("expected 1 data point, got %d", len(dataPoints))
	}

	// Check CORS header
	if cors := rec.Header().Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("expected CORS header '*', got %q", cors)
	}
}

func TestReputationAPI_GetHistory_Empty(t *testing.T) {
	handler, participants, _, _, _, cfg := setupReputationAPITest(t)
	participant, _ := registerTestUser(t, participants, cfg, "rep-empty-hist@example.com", "RepEmptyHist")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/"+participant.ID+"/history", nil)
	req.SetPathValue("id", participant.ID)
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	dataPoints, ok := result["data_points"].([]any)
	if !ok {
		t.Fatal("expected data_points to be an array")
	}
	if len(dataPoints) != 0 {
		t.Errorf("expected 0 data points, got %d", len(dataPoints))
	}
}

func TestReputationAPI_Verify_NotFound(t *testing.T) {
	handler, _, _, _, _, _ := setupReputationAPITest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/00000000-0000-0000-0000-000000000000/verify", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()

	handler.Verify(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestReputationAPI_Verify_Success(t *testing.T) {
	handler, participants, _, _, _, cfg := setupReputationAPITest(t)
	participant, _ := registerTestUser(t, participants, cfg, "rep-verify@example.com", "RepVerifyUser")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation/"+participant.ID+"/verify", nil)
	req.SetPathValue("id", participant.ID)
	rec := httptest.NewRecorder()

	handler.Verify(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var result map[string]any
	testutil.DecodeResponse(t, rec, &result)

	if result["agent_id"] != participant.ID {
		t.Errorf("expected agent_id %q, got %q", participant.ID, result["agent_id"])
	}

	thresholds, ok := result["meets_threshold"].(map[string]any)
	if !ok {
		t.Fatal("expected meets_threshold to be a map")
	}
	// Default trust score is 10, so basic (>=5) should pass, standard (>=15) should fail
	if thresholds["basic"] != true {
		t.Error("expected basic threshold to be true (trust=10 >= 5)")
	}
	if thresholds["standard"] != false {
		t.Error("expected standard threshold to be false (trust=10 < 15)")
	}

	flags, ok := result["flags"].([]any)
	if !ok {
		t.Fatal("expected flags to be an array")
	}
	if len(flags) != 0 {
		t.Errorf("expected 0 flags, got %d", len(flags))
	}

	// Check CORS header
	if cors := rec.Header().Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("expected CORS header '*', got %q", cors)
	}
}

func TestReputationAPI_Verify_MissingID(t *testing.T) {
	handler, _, _, _, _, _ := setupReputationAPITest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reputation//verify", nil)
	req.SetPathValue("id", "")
	rec := httptest.NewRecorder()

	handler.Verify(rec, req)
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}
