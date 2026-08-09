package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupPostTest(t *testing.T) (*handlers.PostHandler, *repository.ParticipantRepo, *repository.CommunityRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "votes", "comments", "posts", "community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants")
	posts := repository.NewPostRepo(pool)
	provenances := repository.NewProvenanceRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewPostHandler(posts, provenances, cfg), participants, communities, cfg
}

// createTestCommunity creates a community for testing and returns it.
func createTestCommunity(t *testing.T, communities *repository.CommunityRepo, createdBy, slug string) *models.Community {
	t.Helper()
	c, err := communities.Create(context.Background(), &models.Community{
		Name:      "Test Community " + slug,
		Slug:      slug,
		CreatedBy: createdBy,
	})
	if err != nil {
		t.Fatalf("creating test community: %v", err)
	}
	return c
}

func TestPostHandler_Create_HumanPost(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	participant, token := registerTestUser(t, participants, cfg, "poster@example.com", "Poster")
	community := createTestCommunity(t, communities, participant.ID, "post-test")

	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Hello World",
		Body:        "This is my first post",
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var post models.Post
	testutil.DecodeResponse(t, rec, &post)

	if post.ID == "" {
		t.Error("expected non-empty post ID")
	}
	if post.Title != "Hello World" {
		t.Errorf("expected title 'Hello World', got %q", post.Title)
	}
	if post.AuthorID != participant.ID {
		t.Errorf("expected author_id %q, got %q", participant.ID, post.AuthorID)
	}
	if post.AuthorType != models.ParticipantHuman {
		t.Errorf("expected author_type 'human', got %q", post.AuthorType)
	}
}

func TestPostHandler_Create_AgentPostWithProvenance(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	owner, _ := registerTestUser(t, participants, cfg, "agent-owner@example.com", "Agent Owner")
	community := createTestCommunity(t, communities, owner.ID, "agent-post-test")

	// Create an agent.
	agent, err := participants.CreateAgent(context.Background(), &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: "TestBot",
		},
		OwnerID:       owner.ID,
		ModelProvider: "openai",
		ModelName:     "gpt-4",
		ProtocolType:  models.ProtocolREST,
	})
	if err != nil {
		t.Fatalf("creating agent: %v", err)
	}

	// Generate a token for the agent.
	agentToken, err := generateTokenForParticipant(cfg, agent.ID, string(models.ParticipantAgent))
	if err != nil {
		t.Fatalf("generating agent token: %v", err)
	}

	confidence := 0.95
	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", agentToken, models.CreatePostRequest{
		CommunityID:     community.ID,
		Title:           "Agent Analysis",
		Body:            "Automated analysis of topic X",
		Sources:         []string{"https://source1.com", "https://source2.com"},
		ConfidenceScore: &confidence,
	})
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var post models.Post
	testutil.DecodeResponse(t, rec, &post)

	if post.ID == "" {
		t.Error("expected non-empty post ID")
	}
	if post.AuthorType != models.ParticipantAgent {
		t.Errorf("expected author_type 'agent', got %q", post.AuthorType)
	}
	if post.ProvenanceID == nil {
		t.Error("expected provenance_id to be set for agent post with sources")
	}
}

func TestPostHandler_Get_Success(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	participant, token := registerTestUser(t, participants, cfg, "getter@example.com", "Getter")
	community := createTestCommunity(t, communities, participant.ID, "get-post-test")

	// Create a post.
	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Get This Post",
		Body:        "Content here",
	})
	createRec := httptest.NewRecorder()
	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create))
	protected.ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	var createdPost models.Post
	testutil.DecodeResponse(t, createRec, &createdPost)

	// Get the post by ID using ServeMux for path value extraction.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/posts/{id}", handler.Get)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+createdPost.ID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	testutil.AssertStatus(t, getRec, http.StatusOK)

	var post models.PostWithAuthor
	testutil.DecodeResponse(t, getRec, &post)

	if post.ID != createdPost.ID {
		t.Errorf("expected post ID %q, got %q", createdPost.ID, post.ID)
	}
	if post.Title != "Get This Post" {
		t.Errorf("expected title 'Get This Post', got %q", post.Title)
	}
}

func TestPostHandler_Get_NotFound(t *testing.T) {
	handler, _, _, _ := setupPostTest(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/posts/{id}", handler.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

// generateTokenForParticipant generates a JWT token for any participant type.
func generateTokenForParticipant(cfg *config.Config, participantID, participantType string) (string, error) {
	return auth.GenerateToken(cfg.JWT.Secret, cfg.JWT.Expiry, participantID, participantType)
}
