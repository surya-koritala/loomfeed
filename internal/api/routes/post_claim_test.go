package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestPostClaimReplaceRouteAuthenticationAndOwnership(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"claim_citations", "post_claims", "api_keys", "posts", "communities",
		"agent_identities", "human_users", "participants",
	)

	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	apiKeys := repository.NewAPIKeyRepo(pool)
	claims := repository.NewPostClaimRepo(pool)

	createHuman := func(suffix string) *models.Participant {
		t.Helper()
		participant, err := participants.CreateHuman(ctx, &models.HumanUser{
			Participant:  models.Participant{DisplayName: "Claims " + suffix},
			Email:        "claims-" + suffix + "@example.com",
			PasswordHash: "test-hash",
		})
		if err != nil {
			t.Fatalf("create human %s: %v", suffix, err)
		}
		return participant
	}

	author := createHuman("author")
	nonAuthor := createHuman("non-author")
	agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
		Participant:       models.Participant{DisplayName: "Claims Agent"},
		OwnerID:           author.ID,
		ModelProvider:     "openai",
		ModelName:         "test-model",
		Capabilities:      []string{"read", "write"},
		MaxRPM:            60,
		ProtocolType:      models.ProtocolREST,
		HeartbeatInterval: 300,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	community, err := communities.Create(ctx, &models.Community{
		Name:      "Claims Route Tests",
		Slug:      "claims-route-tests",
		CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	humanPost, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    author.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Human-authored claims post",
		Body:        "A post used to verify JWT ownership.",
	})
	if err != nil {
		t.Fatalf("create human post: %v", err)
	}
	agentPost, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    agent.ID,
		AuthorType:  models.ParticipantAgent,
		Title:       "Agent-authored claims post",
		Body:        "A post used to verify API-key ownership.",
	})
	if err != nil {
		t.Fatalf("create agent post: %v", err)
	}

	const jwtSecret = "post-claim-route-test-secret"
	authorToken, err := auth.GenerateToken(jwtSecret, time.Hour, author.ID, string(author.Type))
	if err != nil {
		t.Fatalf("generate author JWT: %v", err)
	}
	nonAuthorToken, err := auth.GenerateToken(jwtSecret, time.Hour, nonAuthor.ID, string(nonAuthor.Type))
	if err != nil {
		t.Fatalf("generate non-author JWT: %v", err)
	}
	createAPIKey := func(scopes []string) string {
		t.Helper()
		plain, hash, prefix, err := auth.GenerateAPIKey()
		if err != nil {
			t.Fatalf("generate API key: %v", err)
		}
		_, err = apiKeys.Create(ctx, &models.APIKey{
			AgentID:   agent.ID,
			KeyHash:   hash,
			KeyPrefix: prefix,
			Scopes:    scopes,
			RateLimit: 60,
			ExpiresAt: time.Now().Add(time.Hour),
			IsActive:  true,
		})
		if err != nil {
			t.Fatalf("store API key: %v", err)
		}
		return plain
	}
	emptyScopeKey := createAPIKey([]string{})
	readKey := createAPIKey([]string{"read"})
	writeKey := createAPIKey([]string{"read", "write"})

	mux := http.NewServeMux()
	Register(mux, pool, &config.Config{JWT: config.JWTConfig{Secret: jwtSecret}}, registerOptions{disableBackgroundWorkers: true})
	replace := func(postID, bearerToken, apiKey, claimText string) *httptest.ResponseRecorder {
		t.Helper()
		body := `{"claims":[{"claim_text":"` + claimText + `","citations":[]}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/posts/"+postID+"/claims", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		if apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	t.Run("anonymous is rejected", func(t *testing.T) {
		result := replace(humanPost.ID, "", "", "anonymous claim")
		if result.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
	})

	t.Run("JWT non-author is forbidden", func(t *testing.T) {
		result := replace(humanPost.ID, nonAuthorToken, "", "non-author claim")
		if result.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
	})

	t.Run("JWT author replaces claims", func(t *testing.T) {
		result := replace(humanPost.ID, authorToken, "", "JWT author claim")
		if result.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
		stored, err := claims.ListByPost(ctx, humanPost.ID)
		if err != nil {
			t.Fatalf("list stored claims: %v", err)
		}
		if len(stored) != 1 || stored[0].ClaimText != "JWT author claim" {
			t.Fatalf("stored claims=%+v", stored)
		}
	})

	t.Run("API key without write scope is forbidden", func(t *testing.T) {
		result := replace(agentPost.ID, "", readKey, "read-only key claim")
		if result.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
	})

	t.Run("API key without any scopes is forbidden", func(t *testing.T) {
		result := replace(agentPost.ID, "", emptyScopeKey, "empty-scope key claim")
		if result.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
	})

	t.Run("API key author with write scope replaces claims", func(t *testing.T) {
		result := replace(agentPost.ID, "", writeKey, "API key author claim")
		if result.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
		stored, err := claims.ListByPost(ctx, agentPost.ID)
		if err != nil {
			t.Fatalf("list stored claims: %v", err)
		}
		if len(stored) != 1 || stored[0].ClaimText != "API key author claim" {
			t.Fatalf("stored claims=%+v", stored)
		}
	})
}
