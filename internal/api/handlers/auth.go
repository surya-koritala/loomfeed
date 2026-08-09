package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/ratelimit"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// Per-endpoint rate limiters for auth:
//   - Register: 5 per hour per IP (prevents signup spam)
//   - Login:    10 per minute per IP (supplements per-account lockout)
var (
	registerLimiter = ratelimit.New(5, time.Hour)
	loginLimiter    = ratelimit.New(10, time.Minute)
)

// ResetRateLimiters clears all rate limiter state. For testing only.
func ResetRateLimiters() {
	registerLimiter = ratelimit.New(5, time.Hour)
	loginLimiter = ratelimit.New(10, time.Minute)
}

// ClientIP returns the real client IP for rate-limiting. It delegates to
// middleware.ClientIP, which trusts Cloudflare's CF-Connecting-IP and ignores
// the spoofable left-most X-Forwarded-For. Kept as a thin alias so existing
// handler call sites are unchanged.
func ClientIP(r *http.Request) string {
	return middleware.ClientIP(r)
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	participants  *repository.ParticipantRepo
	refreshTokens *repository.RefreshTokenRepo
	pool          *pgxpool.Pool
	cfg           *config.Config
	cache         *cache.RedisCache

	// Optional — when set, registration credits the inviter with a
	// reputation event via invitee_signed_up.
	invites    *repository.InviteRepo
	reputation *repository.ReputationRepo
}

// WithInvites wires the invite-lookup + reputation repos so the
// register handler can credit an inviter. Both are optional; if
// either is nil the handler silently skips the credit step.
func (h *AuthHandler) WithInvites(invites *repository.InviteRepo, reputation *repository.ReputationRepo) {
	h.invites = invites
	h.reputation = reputation
}

// WithCache sets the Redis cache for JWT token blacklisting.
func (h *AuthHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(participants *repository.ParticipantRepo, refreshTokens *repository.RefreshTokenRepo, pool *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		participants:  participants,
		refreshTokens: refreshTokens,
		pool:          pool,
		cfg:           cfg,
	}
}

// issueAuthCookies sets httpOnly cookies for access + refresh tokens
// when the cookie-issuance flag is on. JSON response body is unchanged
// either way — SDKs and 3rd-party Bearer consumers keep working.
// Phase 1.2 of the security migration; see ~/loomfeed-phase1.2-cookie-auth-plan.md
func (h *AuthHandler) issueAuthCookies(w http.ResponseWriter, access, refresh string) {
	if !h.cfg.Auth.CookieIssuance {
		return
	}
	middleware.SetAuthCookies(w, access, refresh, h.cfg.Environment == "production")
}

// clearAuthCookies expires both auth cookies. Used by Logout.
func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	if !h.cfg.Auth.CookieIssuance {
		return
	}
	middleware.ClearAuthCookies(w, h.cfg.Environment == "production")
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r)
	remaining := registerLimiter.Remaining(ip)
	w.Header().Set("X-RateLimit-Limit", "5")
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	if !registerLimiter.Allow(ip) {
		api.Error(w, http.StatusTooManyRequests, "too many registration attempts, try again later")
		return
	}

	var req models.RegisterRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		api.Error(w, http.StatusBadRequest, "email, password, and display_name are required")
		return
	}

	if len(req.Password) < 8 {
		api.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 128 {
		api.Error(w, http.StatusBadRequest, "password too long")
		return
	}

	if err := api.ValidateEmail(req.Email); err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.DisplayName) > 100 {
		api.Error(w, http.StatusBadRequest, "display_name exceeds 100 character limit")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to process password")
		return
	}

	// Resolve the invite code (if any) BEFORE creating the participant
	// so we can set invited_by atomically. An unknown code is not an
	// error — we don't want signup to fail on a typo — it just means
	// the new account is organic.
	inviterID := ""
	if h.invites != nil && req.InviteCode != "" {
		if id, err := h.invites.LookupCode(r.Context(), req.InviteCode); err == nil {
			inviterID = id
		}
	}

	human := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: req.DisplayName,
			TrustScore:  10,
		},
		Email:                  req.Email,
		PasswordHash:           hash,
		InvitedByParticipantID: inviterID,
	}

	participant, err := h.participants.CreateHuman(r.Context(), human)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			api.Error(w, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("failed to create account", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	// Credit the inviter on successful signup. Failure here shouldn't
	// unwind the signup — the account exists and the link is recorded,
	// the reputation bump is nice-to-have. +1.0 on signup, and (future)
	// an additional +1.5 when the invitee verifies their email.
	if inviterID != "" && h.reputation != nil {
		if err := h.reputation.RecordEvent(r.Context(), inviterID, "invitee_signed_up", 1.0); err != nil {
			slog.Warn("failed to record inviter reputation", "inviter", inviterID, "error", err)
		}
	}

	// Generate short-lived access token (15min)
	accessToken, err := auth.GenerateToken(h.cfg.JWT.Secret, 15*time.Minute, participant.ID, string(participant.Type))
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Generate refresh token
	refreshPlain, refreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	// Store refresh token hash in DB
	if err := h.refreshTokens.Create(r.Context(), participant.ID, refreshHash, auth.RefreshTokenLookupHash(refreshPlain), time.Now().Add(7*24*time.Hour)); err != nil {
		slog.Error("failed to store refresh token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Generate email verification token (24-hour expiry). The token is
	// ONLY delivered via the outbound verification email — never echoed
	// back in this response, as that would expose it in request logs,
	// analytics, and any third-party scripts on the caller's page.
	verificationToken, err := auth.GenerateVerificationToken()
	if err != nil {
		slog.Error("failed to generate verification token", "error", err)
	}
	if verificationToken != "" {
		if err := h.participants.SetVerificationToken(r.Context(), participant.ID, verificationToken, time.Now().Add(24*time.Hour)); err != nil {
			slog.Error("failed to store verification token", "error", err)
		}
	}

	h.issueAuthCookies(w, accessToken, refreshPlain)
	resp := map[string]any{
		"token":         accessToken,
		"access_token":  accessToken,
		"refresh_token": refreshPlain,
		"expires_in":    900,
		"participant":   participant,
	}
	api.JSON(w, http.StatusCreated, resp)
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r)
	remaining := loginLimiter.Remaining(ip)
	w.Header().Set("X-RateLimit-Limit", "10")
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	if !loginLimiter.Allow(ip) {
		api.Error(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req models.LoginRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	human, err := h.participants.GetHumanByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to look up account")
		return
	}

	// Check if account is locked
	if human.LockedUntil != nil && human.LockedUntil.After(time.Now()) {
		api.Error(w, http.StatusTooManyRequests, "account temporarily locked, try again later")
		return
	}

	if !auth.CheckPassword(req.Password, human.PasswordHash) {
		// Increment failed login count; lock after 5 failures
		_, _ = h.pool.Exec(r.Context(), `
			UPDATE human_users SET failed_login_count = failed_login_count + 1,
			    locked_until = CASE WHEN failed_login_count >= 4 THEN NOW() + INTERVAL '15 minutes' ELSE locked_until END
			WHERE participant_id = $1`, human.ID)
		api.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Successful login: reset failed login count, update last_login_at.
	// Also auto-cancel a pending deletion if one was scheduled — the
	// "log in to cancel" flow promised in the deletion email is
	// exactly this path.
	_, _ = h.pool.Exec(r.Context(), `
		UPDATE human_users SET failed_login_count = 0, locked_until = NULL, last_login_at = NOW()
		WHERE participant_id = $1`, human.ID)
	_, _ = h.pool.Exec(r.Context(), `
		UPDATE participants SET pending_deletion_at = NULL
		WHERE id = $1 AND pending_deletion_at IS NOT NULL`, human.ID)

	// Generate short-lived access token (15min)
	accessToken, err := auth.GenerateToken(h.cfg.JWT.Secret, 15*time.Minute, human.ID, string(human.Type))
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Generate refresh token
	refreshPlain, refreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate refresh token")
		return
	}

	// Store refresh token hash in DB
	if err := h.refreshTokens.Create(r.Context(), human.ID, refreshHash, auth.RefreshTokenLookupHash(refreshPlain), time.Now().Add(7*24*time.Hour)); err != nil {
		slog.Error("failed to store refresh token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	h.issueAuthCookies(w, accessToken, refreshPlain)
	api.JSON(w, http.StatusOK, models.AuthResponse{
		Token:        accessToken,
		AccessToken:  accessToken,
		RefreshToken: refreshPlain,
		ExpiresIn:    900, // 15 minutes in seconds
		Participant:  &human.Participant,
	})
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r)
	remaining := loginLimiter.Remaining(ip)
	w.Header().Set("X-RateLimit-Limit", "10")
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	if !loginLimiter.Allow(ip) {
		api.Error(w, http.StatusTooManyRequests, "too many authentication attempts, try again later")
		return
	}

	var req models.RefreshRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prefer the HttpOnly lf_refresh cookie over the JSON body token. With
	// rotation, a multi-tab browser client keeps a stale token in each tab's
	// JS state, but the cookie is shared across tabs and updated atomically by
	// Set-Cookie on every rotation — so reading it first avoids one tab
	// replaying a just-rotated token and tripping reuse detection. The body
	// token remains the path for non-browser/SDK callers.
	refreshTok := middleware.ReadRefreshCookie(r)
	if refreshTok == "" {
		refreshTok = req.RefreshToken
	}
	if refreshTok == "" {
		api.Error(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	// O(1) lookup by the indexed SHA-256 hash instead of bcrypt-scanning
	// every valid token in the table (the old approach was a CPU DoS vector).
	matched, err := h.refreshTokens.FindByLookupHash(r.Context(), auth.RefreshTokenLookupHash(refreshTok))
	if err != nil {
		// No row for this token. (Tokens issued before the lookup-hash
		// migration won't match and fall here — those sessions re-login.)
		api.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Reuse detection: a token that was already rotated/revoked but is being
	// presented again can signal theft (the legitimate client and an attacker
	// both hold it). But a legitimate multi-tab client can also briefly replay
	// a just-rotated token in a race, so only treat reuse as theft — and
	// revoke the whole chain — when it happens OUTSIDE a short grace window.
	// Within the window we just reject this one request; the client already
	// holds the rotated token (cookie + persisted body) and proceeds.
	if matched.RevokedAt != nil {
		const reuseGrace = 30 * time.Second
		if time.Since(*matched.RevokedAt) > reuseGrace {
			_ = h.refreshTokens.RevokeAll(r.Context(), matched.ParticipantID)
			slog.Warn("refresh token reuse detected; revoked all sessions", "participant_id", matched.ParticipantID)
		}
		api.Error(w, http.StatusUnauthorized, "refresh token no longer valid")
		return
	}
	if matched.ExpiresAt.Before(time.Now()) {
		api.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Defense in depth: confirm the bcrypt hash also matches (guards against
	// a SHA-256 collision or a lookup_hash tampering bug).
	if bcrypt.CompareHashAndPassword([]byte(matched.TokenHash), []byte(refreshTok)) != nil {
		api.Error(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Look up participant to get their type
	participant, err := h.participants.GetByID(r.Context(), matched.ParticipantID)
	if err != nil {
		slog.Error("failed to fetch participant for refresh", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to refresh token")
		return
	}

	// Generate new access token
	accessToken, err := auth.GenerateToken(h.cfg.JWT.Secret, 15*time.Minute, participant.ID, string(participant.Type))
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Rotate the refresh token: revoke the presented one and issue a fresh
	// one. A leaked refresh token is then only replayable until its next use,
	// and reuse of the old value triggers the chain-revoke above.
	newRefreshPlain, newRefreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to rotate refresh token")
		return
	}
	if err := h.refreshTokens.Create(r.Context(), participant.ID, newRefreshHash, auth.RefreshTokenLookupHash(newRefreshPlain), time.Now().Add(7*24*time.Hour)); err != nil {
		slog.Error("failed to store rotated refresh token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to refresh token")
		return
	}
	if err := h.refreshTokens.Revoke(r.Context(), matched.ID); err != nil {
		slog.Error("failed to revoke old refresh token", "error", err)
		// Non-fatal: the new token is already issued. Continue.
	}

	// Set both cookies (access + rotated refresh) when cookie issuance is on.
	if h.cfg.Auth.CookieIssuance {
		middleware.SetAuthCookies(w, accessToken, newRefreshPlain, h.cfg.Environment == "production")
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token":         accessToken,
		"refresh_token": newRefreshPlain,
		"expires_in":    900,
	})
}

// Logout handles POST /api/v1/auth/logout (requires auth).
// Revokes all refresh tokens for the authenticated participant.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	if err := h.refreshTokens.RevokeAll(r.Context(), claims.ParticipantID); err != nil {
		slog.Error("failed to revoke refresh tokens", "error", err, "participant_id", claims.ParticipantID)
		api.Error(w, http.StatusInternalServerError, "failed to logout")
		return
	}

	h.clearAuthCookies(w)
	api.JSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// Me handles GET /api/v1/auth/me (requires auth middleware).
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	participant, err := h.participants.GetByID(r.Context(), claims.ParticipantID)
	if err != nil {
		slog.Error("failed to fetch participant", "error", err, "participant_id", claims.ParticipantID)
		api.Error(w, http.StatusInternalServerError, "failed to fetch participant")
		return
	}

	api.JSON(w, http.StatusOK, participant)
}

// ChangePassword handles POST /api/v1/auth/change-password (requires auth).
// Validates the current password, updates to the new one, and blacklists the
// caller's JWT so the old token cannot be reused after the password change.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	var req models.ChangePasswordRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		api.Error(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		api.Error(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > 128 {
		api.Error(w, http.StatusBadRequest, "new password too long")
		return
	}

	// Look up the human user
	human, err := h.participants.GetHumanByParticipantID(r.Context(), claims.ParticipantID)
	if err != nil {
		slog.Error("failed to look up user for password change", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to look up account")
		return
	}

	// Verify current password
	if !auth.CheckPassword(req.CurrentPassword, human.PasswordHash) {
		api.Error(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	// Hash the new password
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to process new password")
		return
	}

	// Update in database
	if err := h.participants.UpdatePassword(r.Context(), claims.ParticipantID, newHash); err != nil {
		slog.Error("failed to update password", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Revoke all refresh tokens so existing sessions can't mint new access tokens
	if err := h.refreshTokens.RevokeAll(r.Context(), claims.ParticipantID); err != nil {
		slog.Error("failed to revoke refresh tokens on password change", "error", err)
	}

	// Blacklist the current access token in Redis so it can't be reused
	tokenString := extractTokenFromHeader(r)
	if tokenString != "" && h.cache != nil {
		blacklistKey := "token_blacklist:" + tokenString
		// Set with TTL matching token expiry (15 minutes max)
		_ = h.cache.Set(r.Context(), blacklistKey, []byte("revoked"), 15*time.Minute)
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

// extractTokenFromHeader extracts the Bearer token from the Authorization header.
func extractTokenFromHeader(r *http.Request) string {
	header := r.Header.Get("Authorization")
	token := strings.TrimPrefix(header, "Bearer ")
	if token == header {
		return ""
	}
	return token
}
