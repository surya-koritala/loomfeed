package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func TestPostHandlerCreateEnforcesQualityGateThresholds(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	pool := database.TestPool(t)
	gates := repository.NewQualityGateRepo(pool)
	handler.WithModeration(repository.NewModerationRepo(pool), communities)
	handler.WithParticipants(participants)
	handler.WithQualityGates(gates)

	human, token := registerTestUser(t, participants, cfg, "quality-gate@example.com", "Quality Author")
	community := createTestCommunity(t, communities, human.ID, "quality-gate-thresholds")

	upsertQualityGate(t, gates, models.QualityGate{
		CommunityID:       community.ID,
		RequireProvenance: true,
	})
	assertPostRejected(t, handler, cfg.JWT.Secret, token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Unsourced",
		Body:        "This human post has no provenance.",
	}, http.StatusForbidden, "requires at least one source")

	upsertQualityGate(t, gates, models.QualityGate{
		CommunityID:        community.ID,
		MinConfidenceScore: 0.8,
	})
	confidence := 0.5
	assertPostRejected(t, handler, cfg.JWT.Secret, token, models.CreatePostRequest{
		CommunityID:     community.ID,
		Title:           "Low confidence",
		Body:            "This confidence is below the community floor.",
		ConfidenceScore: &confidence,
	}, http.StatusForbidden, "minimum confidence")

	upsertQualityGate(t, gates, models.QualityGate{
		CommunityID:   community.ID,
		MinTrustScore: human.TrustScore + 1,
	})
	assertPostRejected(t, handler, cfg.JWT.Secret, token, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Low trust",
		Body:        "This author is below the trust floor.",
	}, http.StatusForbidden, "minimum trust score")
}

func TestPostHandlerCreateEnforcesAgentHourlyLimit(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	pool := database.TestPool(t)
	gates := repository.NewQualityGateRepo(pool)
	handler.WithModeration(repository.NewModerationRepo(pool), communities)
	handler.WithParticipants(participants)
	handler.WithQualityGates(gates)

	owner, _ := registerTestUser(t, participants, cfg, "quality-limit-owner@example.com", "Agent Owner")
	community := createTestCommunity(t, communities, owner.ID, "quality-gate-rate")
	agent, err := participants.CreateAgent(context.Background(), &models.AgentIdentity{
		Participant: models.Participant{DisplayName: "Rate Limited Agent"},
		OwnerID:     owner.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentToken, err := generateTokenForParticipant(cfg, agent.ID, string(models.ParticipantAgent))
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	upsertQualityGate(t, gates, models.QualityGate{CommunityID: community.ID, MaxAgentPostsPerHour: 1})

	createPost := func(title string) *httptest.ResponseRecorder {
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", agentToken, models.CreatePostRequest{
			CommunityID: community.ID,
			Title:       title,
			Body:        "A sourced agent post.",
			Sources:     []string{"https://example.com/source"},
		})
		rec := httptest.NewRecorder()
		middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(rec, req)
		return rec
	}

	testutil.AssertStatus(t, createPost("First post"), http.StatusCreated)
	second := createPost("Second post")
	testutil.AssertStatus(t, second, http.StatusTooManyRequests)
	if !strings.Contains(second.Body.String(), "per hour") {
		t.Fatalf("response should explain hourly gate: %s", second.Body.String())
	}
}

func TestHumanVerificationPublishesGateHeldAgentPost(t *testing.T) {
	handler, participants, communities, cfg := setupPostTest(t)
	pool := database.TestPool(t)
	gates := repository.NewQualityGateRepo(pool)
	posts := repository.NewPostRepo(pool)
	handler.WithModeration(repository.NewModerationRepo(pool), communities)
	handler.WithParticipants(participants)
	handler.WithQualityGates(gates)

	human, humanToken := registerTestUser(t, participants, cfg, "quality-verifier@example.com", "Human Verifier")
	_, secondHumanToken := registerTestUser(t, participants, cfg, "quality-verifier-two@example.com", "Second Human Verifier")
	community := createTestCommunity(t, communities, human.ID, "quality-gate-verification")
	agent, err := participants.CreateAgent(context.Background(), &models.AgentIdentity{
		Participant: models.Participant{DisplayName: "Held Agent"},
		OwnerID:     human.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	agentToken, err := generateTokenForParticipant(cfg, agent.ID, string(models.ParticipantAgent))
	if err != nil {
		t.Fatalf("generate agent token: %v", err)
	}
	upsertQualityGate(t, gates, models.QualityGate{CommunityID: community.ID, RequireHumanVerify: true})

	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", agentToken, models.CreatePostRequest{
		CommunityID: community.ID,
		Title:       "Awaiting a human seal",
		Body:        "A sourced agent post.",
		Sources:     []string{"https://example.com/source"},
	})
	createRec := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)
	var created models.Post
	testutil.DecodeResponse(t, createRec, &created)
	if !created.Quarantined {
		t.Fatal("post should be held from public feeds until human verification")
	}
	if err := posts.SetQuarantined(context.Background(), created.ID, false); !errors.Is(err, repository.ErrHumanVerificationRequired) {
		t.Fatalf("manual release error = %v, want ErrHumanVerificationRequired", err)
	}

	verification := handlers.NewVerificationHandler(repository.NewVerificationRepo(pool), posts, repository.NewReputationRepo(pool))
	webhooks := &sourceEventRecorder{}
	verification.WithWebhook(webhooks)
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/posts/{id}/verify", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(verification.Verify)))
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for _, verifierToken := range []string{humanToken, secondHumanToken} {
		verifierToken := verifierToken
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result := httptest.NewRecorder()
			mux.ServeHTTP(result, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+created.ID+"/verify", verifierToken, nil))
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		testutil.AssertStatus(t, result, http.StatusOK)
	}

	published, err := posts.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get verified post: %v", err)
	}
	if published.Quarantined {
		t.Fatal("first human verification should release the post to public feeds")
	}
	event := webhooks.only(t)
	if event.typeName != webhook.EventPostCreated || event.payload["post_id"] != created.ID || event.payload["title"] != created.Title {
		t.Fatalf("verified publication event=%#v", event)
	}

	// Concurrent and duplicate verifications are idempotent at the publication
	// boundary and must not republish the event.
	again := httptest.NewRecorder()
	mux.ServeHTTP(again, testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts/"+created.ID+"/verify", humanToken, nil))
	testutil.AssertStatus(t, again, http.StatusOK)
	if got := webhooks.count(); got != 1 {
		t.Fatalf("duplicate verification emitted %d publication events, want one", got)
	}

	unverifyMux := http.NewServeMux()
	unverifyMux.Handle("DELETE /api/v1/posts/{id}/verify", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(verification.Unverify)))
	for _, verifierToken := range []string{humanToken, secondHumanToken} {
		unverifyRec := httptest.NewRecorder()
		unverifyMux.ServeHTTP(unverifyRec, testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/posts/"+created.ID+"/verify", verifierToken, nil))
		testutil.AssertStatus(t, unverifyRec, http.StatusOK)
	}
	heldAgain, err := posts.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get unverified post: %v", err)
	}
	if !heldAgain.Quarantined {
		t.Fatal("removing the final human verification should hold the post again")
	}
}

func TestModerationSettingsPersistQualityGate(t *testing.T) {
	_, participants, communities, cfg := setupPostTest(t)
	pool := database.TestPool(t)
	gates := repository.NewQualityGateRepo(pool)
	creator, token := registerTestUser(t, participants, cfg, "quality-admin@example.com", "Quality Admin")
	community := createTestCommunity(t, communities, creator.ID, "quality-gate-settings")

	moderation := handlers.NewModerationHandler(
		repository.NewModerationRepo(pool),
		communities,
		repository.NewReportRepo(pool),
		cfg,
	)
	moderation.WithQualityGates(gates)
	mux := http.NewServeMux()
	mux.Handle("PUT /api/v1/communities/{slug}/settings", middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(moderation.UpdateSettings)))
	req := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/communities/"+community.Slug+"/settings", token, map[string]any{
		"quality_gate": map[string]any{
			"min_trust_score":            42,
			"min_confidence_score":       0.75,
			"require_provenance":         true,
			"require_human_verification": true,
			"max_agent_posts_per_hour":   3,
		},
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	gate, err := gates.GetByCommunityID(context.Background(), community.ID)
	if err != nil {
		t.Fatalf("load saved quality gate: %v", err)
	}
	if gate.MinTrustScore != 42 || gate.MinConfidenceScore != 0.75 || !gate.RequireProvenance ||
		!gate.RequireHumanVerify || gate.MaxAgentPostsPerHour != 3 {
		t.Fatalf("unexpected saved gate: %#v", gate)
	}
}

func upsertQualityGate(t *testing.T, gates *repository.QualityGateRepo, gate models.QualityGate) {
	t.Helper()
	if _, err := gates.Upsert(context.Background(), &gate); err != nil {
		t.Fatalf("upsert quality gate: %v", err)
	}
}

func assertPostRejected(
	t *testing.T,
	handler *handlers.PostHandler,
	jwtSecret, token string,
	request models.CreatePostRequest,
	status int,
	wantMessage string,
) {
	t.Helper()
	req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/posts", token, request)
	rec := httptest.NewRecorder()
	middleware.Auth(jwtSecret)(http.HandlerFunc(handler.Create)).ServeHTTP(rec, req)
	testutil.AssertStatus(t, rec, status)
	if !strings.Contains(rec.Body.String(), wantMessage) {
		t.Fatalf("response should contain %q: %s", wantMessage, rec.Body.String())
	}
}
