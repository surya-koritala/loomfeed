package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// oauthStateCookie is the short-lived HttpOnly cookie that binds the
// OAuth state token to the browser that initiated the flow. SameSite=Lax
// is required so the cookie is sent on the top-level cross-site GET
// redirect from github.com back to the callback URL.
const (
	oauthStateCookie    = "oauth_state_github"
	oauthStateMaxAgeSec = 600 // 10 minutes
)

// OAuthHandler handles OAuth login flows.
type OAuthHandler struct {
	participants *repository.ParticipantRepo
	cfg          *config.Config
}

// NewOAuthHandler creates a new OAuthHandler.
func NewOAuthHandler(participants *repository.ParticipantRepo, cfg *config.Config) *OAuthHandler {
	return &OAuthHandler{participants: participants, cfg: cfg}
}

// GitHubLogin redirects the user to the GitHub OAuth authorization page.
//
// Generates a fresh CSRF state token, stores it in a short-lived
// HttpOnly cookie tied to this browser, and includes it in the URL
// passed to GitHub. GitHubCallback validates the round-trip so an
// attacker can't trick a victim into completing an OAuth flow with
// the attacker's authorization code.
func (h *OAuthHandler) GitHubLogin(w http.ResponseWriter, r *http.Request) {
	clientID := h.cfg.OAuth.GitHubClientID
	if clientID == "" {
		api.Error(w, http.StatusServiceUnavailable, "GitHub OAuth is not configured")
		return
	}

	state, err := generateOAuthState()
	if err != nil {
		slog.Error("failed to generate oauth state", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to start oauth flow")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/api/v1/auth/github",
		MaxAge:   oauthStateMaxAgeSec,
		HttpOnly: true,
		Secure:   isProductionEnv(h.cfg),
		SameSite: http.SameSiteLaxMode,
	})

	redirectURI := h.cfg.OAuth.GitHubRedirectURI
	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=user:email&state=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// GitHubCallback handles the OAuth callback from GitHub.
//
// Validates the state round-trip BEFORE doing anything else with the
// returned code; on mismatch we return 400 without exchanging the code,
// so an attacker who tricks a victim into hitting this endpoint with
// the attacker's auth code never gets the victim logged into the
// attacker's account.
func (h *OAuthHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	receivedState := r.URL.Query().Get("state")
	if receivedState == "" {
		api.Error(w, http.StatusBadRequest, "missing state")
		return
	}

	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || cookie.Value == "" {
		api.Error(w, http.StatusBadRequest, "missing or expired oauth state cookie")
		return
	}

	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(receivedState)) != 1 {
		api.Error(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	// Consume the state cookie — single-use.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/api/v1/auth/github",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isProductionEnv(h.cfg),
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		api.Error(w, http.StatusBadRequest, "missing code")
		return
	}

	// Exchange code for access token
	accessToken, err := exchangeGitHubCode(h.cfg.OAuth.GitHubClientID, h.cfg.OAuth.GitHubClientSecret, code)
	if err != nil {
		slog.Error("failed to exchange github code", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to exchange github code")
		return
	}

	// Fetch user info from GitHub
	ghUser, err := fetchGitHubUser(accessToken)
	if err != nil {
		slog.Error("failed to fetch github user", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to fetch github user info")
		return
	}

	// Always use a GitHub-*verified* email for account matching. The
	// `email` on the /user response is not guaranteed verified, so trusting
	// it would allow account takeover: an attacker sets a victim's email
	// (unverified) on their GitHub profile and links into the victim's
	// account. fetchGitHubPrimaryEmail only returns verified addresses.
	email, err := fetchGitHubPrimaryEmail(accessToken)
	if err != nil || email == "" {
		api.Error(w, http.StatusBadRequest, "could not determine a verified GitHub email address")
		return
	}

	ctx := r.Context()

	// Find or create participant
	existing, err := h.participants.GetHumanByEmail(ctx, email)
	var participantID, participantType string
	if err == nil {
		// Existing account — only allow linking if it was created via
		// GitHub OAuth. Otherwise an attacker proving control of an email
		// at GitHub could log into a same-email password account.
		if existing.OAuthProvider != "github" {
			api.Error(w, http.StatusConflict, "An account with this email already exists. Please log in with your existing method.")
			return
		}
		participantID = existing.ID
		participantType = string(existing.Type)
	} else {
		// Create new human user
		displayName := ghUser.Name
		if displayName == "" {
			displayName = ghUser.Login
		}
		avatarURL := ghUser.AvatarURL

		newHuman := &models.HumanUser{
			Participant: models.Participant{
				DisplayName: displayName,
				AvatarURL:   avatarURL,
			},
			Email:         email,
			PasswordHash:  "", // No password for OAuth users
			OAuthProvider: "github",
		}

		participant, createErr := h.participants.CreateHuman(ctx, newHuman)
		if createErr != nil {
			slog.Error("failed to create github user", "error", createErr)
			api.Error(w, http.StatusInternalServerError, "failed to create user account")
			return
		}
		participantID = participant.ID
		participantType = string(participant.Type)
	}

	// Generate JWT
	token, err := auth.GenerateToken(h.cfg.JWT.Secret, h.cfg.JWT.Expiry, participantID, participantType)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Phase 1.2: set the access cookie alongside the URL token. The
	// URL token is kept during the rollout window so the existing
	// frontend (which reads ?token=...) keeps working; PR B will
	// switch the frontend to the cookie path and a subsequent PR
	// will drop the URL token entirely (it leaks via Referer + logs).
	if h.cfg.Auth.CookieIssuance {
		// OAuth flow doesn't mint a refresh token here, so only the
		// access cookie is set. Refresh comes through the regular
		// /auth/refresh flow on next access-token expiry.
		middleware.SetAccessCookie(w, token, h.cfg.Environment == "production")
	}

	// Pick the frontend URL: prefer the configured public site URL
	// (production), fall back to the dev frontend port for local
	// development. Token still in query param during the rollout —
	// see the cookie note above.
	base := h.cfg.Email.SiteURL
	if base == "" || strings.HasPrefix(base, "http://localhost") {
		base = "http://localhost:5173"
	}
	frontendURL := fmt.Sprintf("%s/login?token=%s", base, url.QueryEscape(token))
	http.Redirect(w, r, frontendURL, http.StatusTemporaryRedirect)
}

// --- GitHub API helpers ---

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func exchangeGitHubCode(clientID, clientSecret, code string) (string, error) {
	body := url.Values{}
	body.Set("client_id", clientID)
	body.Set("client_secret", clientSecret)
	body.Set("code", code)

	req, err := http.NewRequest(http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(body.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("posting to github: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github token error: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGitHubUser(accessToken string) (*githubUser, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling github user api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var user githubUser
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &user); err != nil {
		return nil, fmt.Errorf("decoding github user: %w", err)
	}
	return &user, nil
}

func fetchGitHubPrimaryEmail(accessToken string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling github emails api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decoding github emails: %w", err)
	}
	// Only ever return a GitHub-verified email. Returning an unverified
	// address would let an attacker who set a victim's email (unverified)
	// on their GitHub profile link into the victim's account.
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	// Fall back to any verified email if no verified *primary* exists.
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no verified email on GitHub account")
}

// generateOAuthState returns a cryptographically random base64url-encoded
// state token (32 bytes / 256 bits of entropy).
func generateOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// isProductionEnv reports whether the cookie Secure flag should be set.
// In development the API may be served over plain HTTP; setting Secure
// would silently strip the cookie and break the flow.
func isProductionEnv(cfg *config.Config) bool {
	return cfg.Environment == "production"
}
