package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// EmailSender interface for sending verification emails.
type EmailSender interface {
	SendVerification(to, displayName, token, baseURL string) error
}

// EmailVerifyHandler handles email verification endpoints.
type EmailVerifyHandler struct {
	participants *repository.ParticipantRepo
	emailSender EmailSender
	siteURL     string
}

// WithEmailSender sets the email sender for sending verification emails.
func (h *EmailVerifyHandler) WithEmailSender(sender EmailSender, siteURL string) {
	h.emailSender = sender
	h.siteURL = siteURL
}

// SendVerificationEmail generates a token and sends a verification email.
func (h *EmailVerifyHandler) SendVerificationEmail(participantID, emailAddr, displayName string) {
	token, err := auth.GenerateVerificationToken()
	if err != nil {
		slog.Error("failed to generate verification token", "error", err)
		return
	}
	expires := time.Now().Add(24 * time.Hour)
	if err := h.participants.SetVerificationToken(context.Background(), participantID, token, expires); err != nil {
		slog.Error("failed to set verification token", "error", err)
		return
	}
	if h.emailSender != nil && emailAddr != "" {
		if err := h.emailSender.SendVerification(emailAddr, displayName, token, h.siteURL); err != nil {
			slog.Error("failed to send verification email", "error", err, "to", emailAddr)
		}
	}
}

// NewEmailVerifyHandler creates a new EmailVerifyHandler.
func NewEmailVerifyHandler(participants *repository.ParticipantRepo) *EmailVerifyHandler {
	return &EmailVerifyHandler{participants: participants}
}

// VerifyEmail handles GET /api/v1/auth/verify-email?token=xxx.
// Validates the token, marks the email as verified, and returns a success message.
func (h *EmailVerifyHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		api.Error(w, http.StatusBadRequest, "verification token is required")
		return
	}

	participantID, err := h.participants.VerifyEmail(r.Context(), token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusBadRequest, "invalid or expired verification token")
			return
		}
		slog.Error("failed to verify email", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to verify email")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{
		"status":         "verified",
		"participant_id": participantID,
		"message":        "Email verified successfully",
	})
}

// VerificationStatus handles GET /api/v1/auth/verification-status (auth required).
// Returns whether the authenticated user's email is verified.
func (h *EmailVerifyHandler) VerificationStatus(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	verified, err := h.participants.IsEmailVerified(r.Context(), claims.ParticipantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not a human user (e.g., agent) — treat as not applicable
			api.JSON(w, http.StatusOK, map[string]bool{"verified": false})
			return
		}
		slog.Error("failed to check verification status", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to check verification status")
		return
	}

	api.JSON(w, http.StatusOK, map[string]bool{"verified": verified})
}

// ResendVerification handles POST /api/v1/auth/resend-verification (auth required).
// Generates a new verification token and returns it (actual email sending deferred until SMTP configured).
func (h *EmailVerifyHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	// Check if already verified
	verified, err := h.participants.IsEmailVerified(r.Context(), claims.ParticipantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusBadRequest, "account not found")
			return
		}
		slog.Error("failed to check verification status", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to check verification status")
		return
	}
	if verified {
		api.JSON(w, http.StatusOK, map[string]string{"message": "email already verified"})
		return
	}

	// Generate new token
	token, err := auth.GenerateVerificationToken()
	if err != nil {
		slog.Error("failed to generate verification token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to generate verification token")
		return
	}

	if err := h.participants.SetVerificationToken(r.Context(), claims.ParticipantID, token, time.Now().Add(24*time.Hour)); err != nil {
		slog.Error("failed to store verification token", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to store verification token")
		return
	}

	// Send verification email
	if h.emailSender != nil {
		// Get user email
		user, err := h.participants.GetHumanByParticipantID(r.Context(), claims.ParticipantID)
		if err == nil && user.Email != "" {
			go func() {
				if err := h.emailSender.SendVerification(user.Email, user.DisplayName, token, h.siteURL); err != nil {
					slog.Error("failed to send verification email", "error", err, "to", user.Email)
				} else {
					slog.Info("verification email sent", "to", user.Email)
				}
			}()
		}
	}

	// Token is delivered only via the outbound email; the response body
	// must not echo it back since query-param tokens leak through logs,
	// referrer headers, and third-party analytics on the caller's page.
	api.JSON(w, http.StatusOK, map[string]string{
		"message": "verification email sent",
	})
}
