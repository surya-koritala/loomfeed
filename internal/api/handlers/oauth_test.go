package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/config"
)

// newOAuthTestHandler builds an OAuthHandler suitable for state-validation
// tests. participants is nil — every state-validation failure path returns
// before any participant lookup, so the nil never gets dereferenced. If
// you add a test that successfully passes state validation you'll need a
// real (or stubbed) repo here.
func newOAuthTestHandler() *OAuthHandler {
	cfg := &config.Config{
		Environment: "development",
		OAuth: config.OAuthConfig{
			GitHubClientID:     "test-client-id",
			GitHubClientSecret: "test-client-secret",
			GitHubRedirectURI:  "http://localhost:8080/api/v1/auth/github/callback",
		},
	}
	return NewOAuthHandler(nil, cfg)
}

func TestGitHubLogin_SetsStateCookieAndRedirects(t *testing.T) {
	h := newOAuthTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github", nil)
	rec := httptest.NewRecorder()
	h.GitHubLogin(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusTemporaryRedirect)
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "github.com/login/oauth/authorize") {
		t.Fatalf("redirect should go to github authorize: got %q", loc)
	}
	if !strings.Contains(loc, "state=") {
		t.Fatalf("redirect URL must include state param: got %q", loc)
	}

	cookies := rec.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == oauthStateCookie {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatalf("expected state cookie %q to be set", oauthStateCookie)
	}
	if stateCookie.Value == "" {
		t.Fatalf("state cookie value must not be empty")
	}
	if !stateCookie.HttpOnly {
		t.Errorf("state cookie should be HttpOnly")
	}
	if stateCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("state cookie SameSite should be Lax, got %v", stateCookie.SameSite)
	}
	if stateCookie.MaxAge != oauthStateMaxAgeSec {
		t.Errorf("state cookie MaxAge: got %d, want %d", stateCookie.MaxAge, oauthStateMaxAgeSec)
	}
}

func TestGitHubLogin_GeneratesUniqueStatePerCall(t *testing.T) {
	h := newOAuthTestHandler()

	values := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github", nil)
		rec := httptest.NewRecorder()
		h.GitHubLogin(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == oauthStateCookie {
				if _, dup := values[c.Value]; dup {
					t.Fatalf("state %q was generated twice in 10 calls — entropy check failed", c.Value)
				}
				values[c.Value] = struct{}{}
			}
		}
	}
	if len(values) != 10 {
		t.Fatalf("expected 10 unique states, got %d", len(values))
	}
}

func TestGitHubCallback_RejectsMissingState(t *testing.T) {
	h := newOAuthTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?code=abc", nil)
	rec := httptest.NewRecorder()
	h.GitHubCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing state, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestGitHubCallback_RejectsMissingCookie(t *testing.T) {
	h := newOAuthTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?code=abc&state=some-state", nil)
	rec := httptest.NewRecorder()
	h.GitHubCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when state cookie is absent, got %d", rec.Code)
	}
}

func TestGitHubCallback_RejectsMismatchedState(t *testing.T) {
	h := newOAuthTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?code=abc&state=attacker-supplied", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: "victim-real-state"})
	rec := httptest.NewRecorder()
	h.GitHubCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on state mismatch, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid oauth state") {
		t.Errorf("error body should say 'invalid oauth state', got: %s", rec.Body.String())
	}
}

func TestGitHubCallback_RejectsEmptyCookieValue(t *testing.T) {
	h := newOAuthTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/github/callback?code=abc&state=anything", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: ""})
	rec := httptest.NewRecorder()
	h.GitHubCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on empty cookie, got %d", rec.Code)
	}
}

func TestGenerateOAuthState_HasEnoughEntropy(t *testing.T) {
	const minLen = 40 // 32 bytes base64url encoded ≈ 43 chars
	s, err := generateOAuthState()
	if err != nil {
		t.Fatalf("generateOAuthState: %v", err)
	}
	if len(s) < minLen {
		t.Errorf("state too short: %d < %d (got %q)", len(s), minLen, s)
	}
}
