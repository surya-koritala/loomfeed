package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// GoogleAuthHandler handles Google Sign-In authentication.
type GoogleAuthHandler struct {
	participants  *repository.ParticipantRepo
	refreshTokens *repository.RefreshTokenRepo
	cfg           *config.Config
}

// NewGoogleAuthHandler creates a new GoogleAuthHandler.
func NewGoogleAuthHandler(participants *repository.ParticipantRepo, refreshTokens *repository.RefreshTokenRepo, cfg *config.Config) *GoogleAuthHandler {
	return &GoogleAuthHandler{participants: participants, refreshTokens: refreshTokens, cfg: cfg}
}

type googleTokenInfo struct {
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Iss           string `json:"iss"`
}

// googleTokenInfoClient bounds the outbound call to Google's tokeninfo
// endpoint. http.Get uses no timeout, so a slow/hung response would block
// the auth handler goroutine indefinitely.
var googleTokenInfoClient = &http.Client{Timeout: 10 * time.Second}

func verifyGoogleToken(credential string) (*googleTokenInfo, error) {
	resp, err := googleTokenInfoClient.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(credential))
	if err != nil {
		return nil, fmt.Errorf("failed to contact Google: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Google response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google rejected token: %s", string(body))
	}
	var info googleTokenInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("failed to parse Google response: %w", err)
	}
	return &info, nil
}

// Auth handles POST /api/v1/auth/google.
func (h *GoogleAuthHandler) Auth(w http.ResponseWriter, r *http.Request) {
	if h.cfg.GoogleClientID == "" {
		api.Error(w, http.StatusNotImplemented, "Google Sign-In not configured")
		return
	}

	var req struct {
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Credential == "" {
		api.Error(w, http.StatusBadRequest, "credential is required")
		return
	}

	info, err := verifyGoogleToken(req.Credential)
	if err != nil {
		slog.Error("google auth: token verification failed", "error", err)
		api.Error(w, http.StatusUnauthorized, "invalid Google token")
		return
	}

	if info.Aud != h.cfg.GoogleClientID {
		api.Error(w, http.StatusUnauthorized, "invalid client")
		return
	}

	// The issuer must be Google. tokeninfo validates signature/expiry, but
	// asserting iss explicitly rejects tokens minted by another issuer for
	// the same audience.
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		api.Error(w, http.StatusUnauthorized, "invalid token issuer")
		return
	}

	if info.EmailVerified != "true" {
		api.Error(w, http.StatusUnauthorized, "email not verified")
		return
	}

	email := strings.ToLower(info.Email)

	existing, err := h.participants.GetHumanByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("google auth: db lookup failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	var participant *models.Participant
	isNewUser := false

	if existing != nil {
		// Account exists — ensure it was created via Google OAuth
		if existing.OAuthProvider != "google" {
			api.Error(w, http.StatusConflict, "An account with this email already exists. Please log in with your password.")
			return
		}
		participant = &existing.Participant
	} else {
		// New user — apply rate limiting on registration
		ip := ClientIP(r)
		if !registerLimiter.Allow(ip) {
			api.Error(w, http.StatusTooManyRequests, "too many registration attempts, try again later")
			return
		}

		displayName := info.Name
		if displayName == "" {
			displayName = strings.Split(email, "@")[0]
		}

		human := &models.HumanUser{
			Participant: models.Participant{
				DisplayName: displayName,
				TrustScore:  10,
			},
			Email:         email,
			PasswordHash:  "",
			OAuthProvider: "google",
		}

		participant, err = h.participants.CreateHuman(r.Context(), human)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				api.Error(w, http.StatusConflict, "email already registered")
				return
			}
			slog.Error("google auth: failed to create account", "error", err)
			api.Error(w, http.StatusInternalServerError, "failed to create account")
			return
		}
		isNewUser = true
	}

	accessToken, err := auth.GenerateToken(h.cfg.JWT.Secret, 15*time.Minute, participant.ID, string(participant.Type))
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshPlain, refreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	if err := h.refreshTokens.Create(r.Context(), participant.ID, refreshHash, auth.RefreshTokenLookupHash(refreshPlain), time.Now().Add(7*24*time.Hour)); err != nil {
		slog.Error("google auth: failed to store refresh token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	status := http.StatusOK
	if isNewUser {
		status = http.StatusCreated
	}

	if h.cfg.Auth.CookieIssuance {
		middleware.SetAuthCookies(w, accessToken, refreshPlain, h.cfg.Environment == "production")
	}
	api.JSON(w, status, map[string]any{
		"token":         accessToken,
		"access_token":  accessToken,
		"refresh_token": refreshPlain,
		"expires_in":    900,
		"participant":   participant,
	})
}
