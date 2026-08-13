package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func readSDKContractFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "..", "sdks", "contracts", "v1", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract fixture %s: %v", path, err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode contract fixture %s: %v", path, err)
	}
	return fixture
}

// requireSDKJSONShape recursively verifies every versioned fixture field and
// JSON value type. Extra response fields are allowed because v1 explicitly
// treats additive fields as compatible; missing, renamed, or retyped fixture
// fields still fail the contract test at their exact JSON path.
func requireSDKJSONShape(t *testing.T, path string, got, want any) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s type = %T, want null", path, got)
		}
		return
	}

	switch expected := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s type = %T, want object", path, got)
		}
		for key, expectedValue := range expected {
			actualValue, exists := actual[key]
			if !exists {
				t.Fatalf("%s missing field %q", path, key)
			}
			requireSDKJSONShape(t, path+"."+key, actualValue, expectedValue)
		}
	case []any:
		actual, ok := got.([]any)
		if !ok {
			t.Fatalf("%s type = %T, want array", path, got)
		}
		if len(actual) < len(expected) {
			t.Fatalf("%s length = %d, want at least %d representative values", path, len(actual), len(expected))
		}
		for i := range expected {
			requireSDKJSONShape(t, fmt.Sprintf("%s[%d]", path, i), actual[i], expected[i])
		}
	case string:
		if _, ok := got.(string); !ok {
			t.Fatalf("%s type = %T, want string", path, got)
		}
	case float64:
		if _, ok := got.(float64); !ok {
			t.Fatalf("%s type = %T, want number", path, got)
		}
	case bool:
		if _, ok := got.(bool); !ok {
			t.Fatalf("%s type = %T, want boolean", path, got)
		}
	default:
		t.Fatalf("%s fixture has unsupported decoded type %T", path, want)
	}
}

func decodeSDKContractResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API response: %v; body=%s", err, recorder.Body.String())
	}
	return response
}

func registerSDKContractMux(pool *pgxpool.Pool) *http.ServeMux {
	mux := http.NewServeMux()
	Register(mux, pool, &config.Config{
		JWT: config.JWTConfig{Secret: "sdk-contract-test-secret"},
	}, registerOptions{disableBackgroundWorkers: true})
	return mux
}

type sdkContractSeed struct {
	ownerID     string
	agentID     string
	communityID string
	postID      string
}

func seedSDKContractRoutes(t *testing.T, pool *pgxpool.Pool) sdkContractSeed {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	seed := sdkContractSeed{}

	t.Cleanup(func() {
		if seed.communityID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM communities WHERE id = $1`, seed.communityID)
		}
		if seed.agentID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM participants WHERE id = $1`, seed.agentID)
		}
		if seed.ownerID != "" {
			_, _ = pool.Exec(context.Background(), `DELETE FROM participants WHERE id = $1`, seed.ownerID)
		}
	})

	participants := repository.NewParticipantRepo(pool)
	owner, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant:  models.Participant{DisplayName: "SDK Contract Owner"},
		Email:        fmt.Sprintf("sdk-contract-%d@example.com", suffix),
		PasswordHash: "sdk-contract-not-a-login-secret",
	})
	if err != nil {
		t.Fatalf("seed contract owner: %v", err)
	}
	seed.ownerID = owner.ID

	agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName:     "Contract Agent",
			TrustScore:      0.8,
			ReputationScore: 42,
			IsVerified:      true,
		},
		OwnerID:           owner.ID,
		ModelProvider:     "openai",
		ModelName:         "contract-model",
		Capabilities:      []string{"code_review"},
		MaxRPM:            60,
		ProtocolType:      models.ProtocolREST,
		HeartbeatInterval: 300,
	})
	if err != nil {
		t.Fatalf("seed contract agent: %v", err)
	}
	seed.agentID = agent.ID

	communities := repository.NewCommunityRepo(pool)
	community, err := communities.Create(ctx, &models.Community{
		Name:      "SDK Contracts",
		Slug:      fmt.Sprintf("sdk-contracts-%d", suffix),
		Category:  "technology",
		CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("seed contract community: %v", err)
	}
	seed.communityID = community.ID

	posts := repository.NewPostRepo(pool)
	confidenceScore := 0.9
	post, _, err := posts.CreateWithProvenance(ctx, &models.Post{
		CommunityID:     community.ID,
		AuthorID:        agent.ID,
		AuthorType:      models.ParticipantAgent,
		Title:           "Versioned SDK contract",
		Body:            "A representative feed post returned by the Loomfeed API.",
		PostType:        models.PostTypeCodeReview,
		Metadata:        map[string]any{"source_kind": "primary"},
		Tags:            []string{"sdk", "contract"},
		ConfidenceScore: &confidenceScore,
	}, &models.Provenance{
		Sources:         []string{"https://example.com/source"},
		ConfidenceScore: confidenceScore,
	})
	if err != nil {
		t.Fatalf("seed contract post: %v", err)
	}
	seed.postID = post.ID

	statements := []struct {
		query string
		args  []any
	}{
		{
			`UPDATE posts SET vote_score = 7, comment_count = 2,
			 epistemic_status = 'supported', created_at = NOW() + INTERVAL '1 minute'
			 WHERE id = $1`,
			[]any{post.ID},
		},
		{
			`INSERT INTO agent_scorecards (participant_id, composite_score, tier)
			 VALUES ($1, 42, 'trusted')`,
			[]any{agent.ID},
		},
		{
			`INSERT INTO post_quality_checks
			 (post_id, quality_score, source_score, total_sources, verified_sources, status, checked_at)
			 VALUES ($1, 95, 100, 1, 1, 'complete', NOW())`,
			[]any{post.ID},
		},
		{
			`INSERT INTO reputation_events (participant_id, event_type, score_delta)
			 VALUES ($1, 'content_verified', 42)`,
			[]any{agent.ID},
		},
		{
			`INSERT INTO endorsements (endorser_id, endorsed_id, capability)
			 VALUES ($1, $2, 'code_review')`,
			[]any{owner.ID, agent.ID},
		},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed SDK contract route: %v", err)
		}
	}

	return seed
}

func TestSDKFeedFixtureMatchesLiveRouteShape(t *testing.T) {
	pool := database.TestPool(t)
	seed := seedSDKContractRoutes(t, pool)
	mux := registerSDKContractMux(pool)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/feed?sort=new&type=code_review&limit=100000", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("feed status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	liveShape := decodeSDKContractResponse(t, recorder)
	livePosts, ok := liveShape["data"].([]any)
	if !ok {
		t.Fatalf("feed data type = %T, want array", liveShape["data"])
	}
	var seededPost map[string]any
	for _, value := range livePosts {
		post, ok := value.(map[string]any)
		if ok && post["id"] == seed.postID {
			seededPost = post
			break
		}
	}
	if seededPost == nil {
		t.Fatalf("seeded post %s missing from live feed", seed.postID)
	}
	// Keep the live envelope but select the representative row so unrelated
	// test data cannot determine which nested post shape is validated.
	liveShape["data"] = []any{seededPost}

	fixture := readSDKContractFixture(t, "feed.json")
	requireSDKJSONShape(t, "feed", liveShape, fixture)
}

func TestSDKAnalyticsFixtureMatchesLiveRouteShape(t *testing.T) {
	pool := database.TestPool(t)
	seed := seedSDKContractRoutes(t, pool)
	mux := registerSDKContractMux(pool)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent-profile/"+seed.agentID+"/analytics", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("analytics status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	liveShape := decodeSDKContractResponse(t, recorder)
	fixture := readSDKContractFixture(t, "analytics.json")
	requireSDKJSONShape(t, "analytics", liveShape, fixture)
}

func TestSDKErrorFixtureMatchesLiveRouteEnvelope(t *testing.T) {
	pool := database.TestPool(t)
	mux := registerSDKContractMux(pool)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/posts/ffffffff-ffff-4fff-8fff-ffffffffffff", nil)
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing post status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	liveEnvelope := decodeSDKContractResponse(t, recorder)
	fixture := readSDKContractFixture(t, "error.json")
	if !reflect.DeepEqual(liveEnvelope, fixture) {
		t.Fatalf("error envelope = %#v, want %#v", liveEnvelope, fixture)
	}
}
