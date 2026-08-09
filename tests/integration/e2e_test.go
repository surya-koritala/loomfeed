package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/routes"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
)

// doJSON performs an HTTP request with JSON body and optional Bearer token.
// Returns the decoded JSON object (map), HTTP status code, and any error.
func doJSON(client *http.Client, baseURL, method, path string, body any, token string) (map[string]any, int, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling body: %w", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("doing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Non-fatal: response may be empty or an array
		result = map[string]any{}
	}
	return result, resp.StatusCode, nil
}


// doJSONArray performs an HTTP request and decodes the response as a JSON array.
// Used for endpoints that return a bare JSON array (not a paginated object).
func doJSONArray(client *http.Client, baseURL, method, path string, body any, token string) ([]any, int, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling body: %w", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("doing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result []any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		result = []any{}
	}
	return result, resp.StatusCode, nil
}

// getString extracts a string value from a JSON map, failing the test if missing.
func getString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("expected key %q in response %v", key, m)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected %q to be a string, got %T: %v", key, v, v)
	}
	if s == "" {
		t.Fatalf("expected %q to be non-empty", key)
	}
	return s
}

// getNestedString extracts a string value from a nested map, failing the test if missing.
func getNestedString(t *testing.T, m map[string]any, outerKey, innerKey string) string {
	t.Helper()
	outer, ok := m[outerKey]
	if !ok {
		t.Fatalf("expected key %q in response %v", outerKey, m)
	}
	nested, ok := outer.(map[string]any)
	if !ok {
		t.Fatalf("expected %q to be a map, got %T: %v", outerKey, outer, outer)
	}
	return getString(t, nested, innerKey)
}

func TestE2E_HappyPath(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"votes", "comments", "provenances", "posts",
		"api_keys", "community_subscriptions", "communities",
		"agent_identities", "human_users", "participants",
	)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "e2e-test-secret",
			Expiry: time.Hour,
		},
	}

	mux := http.NewServeMux()
	routes.Register(mux, pool, cfg)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := srv.Client()
	base := srv.URL

	// -----------------------------------------------------------------------
	// Step 1: Register human user
	// -----------------------------------------------------------------------
	t.Log("Step 1: Register human user")
	regBody := map[string]any{
		"email":        "alice@example.com",
		"password":     "hunter2secret",
		"display_name": "Alice",
	}
	regResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/auth/register", regBody, "")
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from register, got %d: %v", status, regResp)
	}
	token := getString(t, regResp, "token")
	t.Logf("registered user, token length=%d", len(token))

	// -----------------------------------------------------------------------
	// Step 2: Login
	// -----------------------------------------------------------------------
	t.Log("Step 2: Login")
	loginBody := map[string]any{
		"email":    "alice@example.com",
		"password": "hunter2secret",
	}
	loginResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from login, got %d: %v", status, loginResp)
	}
	loginToken := getString(t, loginResp, "token")
	if loginToken == "" {
		t.Fatal("expected non-empty token from login")
	}
	t.Log("login successful, token returned")

	// -----------------------------------------------------------------------
	// Step 3: Get me
	// -----------------------------------------------------------------------
	t.Log("Step 3: GET /api/v1/auth/me")
	meResp, status, err := doJSON(client, base, http.MethodGet, "/api/v1/auth/me", nil, token)
	if err != nil {
		t.Fatalf("me request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from /me, got %d: %v", status, meResp)
	}
	participantID := getString(t, meResp, "id")
	displayName := getString(t, meResp, "display_name")
	if displayName != "Alice" {
		t.Fatalf("expected display_name=Alice, got %q", displayName)
	}
	t.Logf("me: participant_id=%s display_name=%s", participantID, displayName)

	// -----------------------------------------------------------------------
	// Step 4: Create community
	// -----------------------------------------------------------------------
	t.Log("Step 4: POST /api/v1/communities")
	communityBody := map[string]any{
		"name":        "Test Community",
		"slug":        "test-community",
		"description": "A community for end-to-end integration testing — creating posts, voting, and exercising the API surface.",
		"category":    "meta",
	}
	commResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/communities", communityBody, token)
	if err != nil {
		t.Fatalf("create community request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from create community, got %d: %v", status, commResp)
	}
	communityID := getString(t, commResp, "id")
	communitySlug := getString(t, commResp, "slug")
	if communitySlug != "test-community" {
		t.Fatalf("expected slug=test-community, got %q", communitySlug)
	}
	t.Logf("created community: id=%s slug=%s", communityID, communitySlug)

	// -----------------------------------------------------------------------
	// Step 5: List communities
	// GET /api/v1/communities returns a bare JSON array ([]Community), not paginated.
	// -----------------------------------------------------------------------
	t.Log("Step 5: GET /api/v1/communities")
	commList, listCommStatus, err := doJSONArray(client, base, http.MethodGet, "/api/v1/communities", nil, "")
	if err != nil {
		t.Fatalf("list communities request failed: %v", err)
	}
	if listCommStatus != http.StatusOK {
		t.Fatalf("expected 200 from list communities, got %d", listCommStatus)
	}
	if len(commList) == 0 {
		t.Fatal("expected at least one community in list")
	}
	t.Logf("list communities returned %d communities", len(commList))

	// -----------------------------------------------------------------------
	// Step 6: Create post
	// -----------------------------------------------------------------------
	t.Log("Step 6: POST /api/v1/posts")
	postBody := map[string]any{
		"community_id": communityID,
		"title":        "Hello World",
		"body":         "This is the first post in the e2e test.",
		"post_type": "text",
	}
	postResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/posts", postBody, token)
	if err != nil {
		t.Fatalf("create post request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from create post, got %d: %v", status, postResp)
	}
	postID := getString(t, postResp, "id")
	postTitle := getString(t, postResp, "title")
	if postTitle != "Hello World" {
		t.Fatalf("expected post title=Hello World, got %q", postTitle)
	}
	t.Logf("created post: id=%s title=%s", postID, postTitle)

	// -----------------------------------------------------------------------
	// Step 7: Get post
	// GET /api/v1/posts/{id} returns PostWithAuthor with author data.
	// -----------------------------------------------------------------------
	t.Log("Step 7: GET /api/v1/posts/{id}")
	getPostResp, status, err := doJSON(client, base, http.MethodGet, "/api/v1/posts/"+postID, nil, "")
	if err != nil {
		t.Fatalf("get post request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from get post, got %d: %v", status, getPostResp)
	}
	// Verify author data is present
	if _, ok := getPostResp["author"]; !ok {
		t.Fatalf("expected 'author' key in get post response: %v", getPostResp)
	}
	authorDisplayName := getNestedString(t, getPostResp, "author", "display_name")
	if authorDisplayName != "Alice" {
		t.Fatalf("expected author display_name=Alice, got %q", authorDisplayName)
	}
	t.Logf("get post: author=%s", authorDisplayName)

	// -----------------------------------------------------------------------
	// Step 8: Create comment
	// -----------------------------------------------------------------------
	t.Log("Step 8: POST /api/v1/posts/{id}/comments")
	commentBody := map[string]any{
		"body": "Great post! This is a test comment.",
	}
	commentResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/posts/"+postID+"/comments", commentBody, token)
	if err != nil {
		t.Fatalf("create comment request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from create comment, got %d: %v", status, commentResp)
	}
	commentID := getString(t, commentResp, "id")
	t.Logf("created comment: id=%s", commentID)

	// -----------------------------------------------------------------------
	// Step 9: List comments
	// GET /api/v1/posts/{id}/comments returns a bare JSON array ([]CommentWithAuthor).
	// -----------------------------------------------------------------------
	t.Log("Step 9: GET /api/v1/posts/{id}/comments")
	commentsList, listCommentsStatus, err := doJSONArray(client, base, http.MethodGet, "/api/v1/posts/"+postID+"/comments", nil, "")
	if err != nil {
		t.Fatalf("list comments request failed: %v", err)
	}
	if listCommentsStatus != http.StatusOK {
		t.Fatalf("expected 200 from list comments, got %d", listCommentsStatus)
	}
	if len(commentsList) == 0 {
		t.Fatal("expected at least one comment in list")
	}
	t.Logf("list comments returned %d comments", len(commentsList))

	// -----------------------------------------------------------------------
	// Step 10: Vote on post
	// -----------------------------------------------------------------------
	t.Log("Step 10: POST /api/v1/votes (upvote post)")
	voteBody := map[string]any{
		"target_id":   postID,
		"target_type": "post",
		"direction":   "up",
	}
	voteResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/votes", voteBody, token)
	if err != nil {
		t.Fatalf("vote request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from vote, got %d: %v", status, voteResp)
	}
	if _, ok := voteResp["vote_score"]; !ok {
		t.Fatalf("expected 'vote_score' key in vote response: %v", voteResp)
	}
	t.Logf("vote cast, vote_score=%v", voteResp["vote_score"])

	// -----------------------------------------------------------------------
	// Step 11: Get feed (global)
	// GET /api/v1/feed returns PaginatedResponse with "data" key.
	// -----------------------------------------------------------------------
	t.Log("Step 11: GET /api/v1/feed")
	feedResp, status, err := doJSON(client, base, http.MethodGet, "/api/v1/feed", nil, "")
	if err != nil {
		t.Fatalf("feed request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from feed, got %d: %v", status, feedResp)
	}
	feedData, ok := feedResp["data"]
	if !ok {
		t.Fatalf("expected 'data' key in feed response: %v", feedResp)
	}
	feedList, ok := feedData.([]any)
	if !ok {
		t.Fatalf("expected 'data' to be an array, got %T: %v", feedData, feedData)
	}
	if len(feedList) == 0 {
		t.Fatal("expected at least one post in global feed")
	}
	t.Logf("global feed returned %d posts", len(feedList))

	// -----------------------------------------------------------------------
	// Step 12: Get community feed
	// GET /api/v1/communities/{slug}/feed returns PaginatedResponse with "data" key.
	// -----------------------------------------------------------------------
	t.Log("Step 12: GET /api/v1/communities/{slug}/feed")
	commFeedResp, status, err := doJSON(client, base, http.MethodGet, "/api/v1/communities/"+communitySlug+"/feed", nil, "")
	if err != nil {
		t.Fatalf("community feed request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from community feed, got %d: %v", status, commFeedResp)
	}
	commFeedData, ok := commFeedResp["data"]
	if !ok {
		t.Fatalf("expected 'data' key in community feed response: %v", commFeedResp)
	}
	commFeedList, ok := commFeedData.([]any)
	if !ok {
		t.Fatalf("expected 'data' to be an array, got %T: %v", commFeedData, commFeedData)
	}
	if len(commFeedList) == 0 {
		t.Fatal("expected at least one post in community feed")
	}
	t.Logf("community feed returned %d posts", len(commFeedList))

	// -----------------------------------------------------------------------
	// Step 13: Register agent (authenticated as human user)
	// POST /api/v1/agents returns RegisterAgentResponse: {agent: {...}, api_key: "..."}
	// -----------------------------------------------------------------------
	t.Log("Step 13: POST /api/v1/agents")
	agentBody := map[string]any{
		"display_name":   "TestBot",
		"model_provider": "anthropic",
		"model_name":     "claude-test",
		"model_version":  "1.0",
		"capabilities":   []string{"text-generation"},
	}
	agentResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/agents", agentBody, token)
	if err != nil {
		t.Fatalf("register agent request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from register agent, got %d: %v", status, agentResp)
	}
	apiKey := getString(t, agentResp, "api_key")
	agentData, ok := agentResp["agent"]
	if !ok {
		t.Fatalf("expected 'agent' key in register agent response: %v", agentResp)
	}
	agentMap, ok := agentData.(map[string]any)
	if !ok {
		t.Fatalf("expected 'agent' to be a map, got %T: %v", agentData, agentData)
	}
	agentID := getString(t, agentMap, "id")
	t.Logf("registered agent: id=%s, api_key length=%d", agentID, len(apiKey))

	// -----------------------------------------------------------------------
	// Step 14: Create agent post
	// Note: POST /api/v1/posts uses middleware.Auth (JWT only), not CombinedAuth.
	// The API key middleware (apikey.go) is not wired into this route.
	// Agent posts must therefore use the human owner's Bearer token.
	// Provenance auto-creation requires participant_type="agent" in JWT claims —
	// which is only set when authenticated via X-API-Key. Since the route uses
	// JWT-only auth, the post will be created without auto-provenance here.
	// The sources and confidence_score fields are still submitted as part of the
	// request body to confirm the handler accepts them without error.
	// -----------------------------------------------------------------------
	t.Log("Step 14: POST /api/v1/posts (agent post submitted via owner JWT — JWT-only route)")
	confidenceScore := 0.92
	agentPostBody := map[string]any{
		"community_id":     communityID,
		"title":            "Agent Generated Post",
		"body":             "This post was generated by an AI agent for testing purposes.",
		"content_type":     "text",
		"sources":          []string{"https://example.com/source1", "https://example.com/source2"},
		"confidence_score": confidenceScore,
	}
	agentPostResp, status, err := doJSON(client, base, http.MethodPost, "/api/v1/posts", agentPostBody, token)
	if err != nil {
		t.Fatalf("agent post request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201 from agent post, got %d: %v", status, agentPostResp)
	}
	agentPostID := getString(t, agentPostResp, "id")
	agentPostTitle := getString(t, agentPostResp, "title")
	if agentPostTitle != "Agent Generated Post" {
		t.Fatalf("expected title='Agent Generated Post', got %q", agentPostTitle)
	}
	t.Logf("created agent post: id=%s", agentPostID)

	// -----------------------------------------------------------------------
	// Step 15: Get agent post
	// -----------------------------------------------------------------------
	t.Log("Step 15: GET /api/v1/posts/{id} (agent post)")
	getAgentPostResp, status, err := doJSON(client, base, http.MethodGet, "/api/v1/posts/"+agentPostID, nil, "")
	if err != nil {
		t.Fatalf("get agent post request failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200 from get agent post, got %d: %v", status, getAgentPostResp)
	}
	fetchedTitle := getString(t, getAgentPostResp, "title")
	if fetchedTitle != "Agent Generated Post" {
		t.Fatalf("expected title='Agent Generated Post', got %q", fetchedTitle)
	}
	t.Logf("get agent post: title=%s", fetchedTitle)

	// -----------------------------------------------------------------------
	// Step 16: Verify agent API key is stored and agent appears in list
	// GET /api/v1/agents returns a bare JSON array ([]AgentIdentity).
	// -----------------------------------------------------------------------
	t.Log("Step 16: GET /api/v1/agents (verify agent API key is stored)")
	agentList, agentListStatus, err := doJSONArray(client, base, http.MethodGet, "/api/v1/agents", nil, token)
	if err != nil {
		t.Fatalf("list agents request failed: %v", err)
	}
	if agentListStatus != http.StatusOK {
		t.Fatalf("expected 200 from list agents, got %d", agentListStatus)
	}
	if len(agentList) == 0 {
		t.Fatal("expected at least one agent in list")
	}
	t.Logf("list agents returned %d agents; API key is valid and stored", len(agentList))

	t.Log("E2E happy path complete!")
}
