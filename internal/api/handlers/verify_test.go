package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupVerifyTest(t *testing.T) (*handlers.EmailVerifyHandler, *handlers.AuthHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	handlers.ResetRateLimiters()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "refresh_tokens", "human_users", "participants")
	participants := repository.NewParticipantRepo(pool)
	refreshTokens := repository.NewRefreshTokenRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	emailVerifyH := handlers.NewEmailVerifyHandler(participants)
	authH := handlers.NewAuthHandler(participants, refreshTokens, pool, cfg)
	return emailVerifyH, authH, participants, cfg
}

// registerUser is a helper that registers a user and returns the participant.
func registerUser(t *testing.T, authH *handlers.AuthHandler, email, password, displayName string) *models.Participant {
	t.Helper()
	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       email,
		Password:    password,
		DisplayName: displayName,
	})
	rec := httptest.NewRecorder()
	authH.Register(rec, req)
	testutil.AssertStatus(t, rec, http.StatusCreated)

	var resp map[string]any
	testutil.DecodeResponse(t, rec, &resp)

	// Extract participant from response
	participantMap := resp["participant"].(map[string]any)
	return &models.Participant{
		ID:          participantMap["id"].(string),
		DisplayName: participantMap["display_name"].(string),
	}
}

func TestEmailVerifyHandler_VerifyEmail_Success(t *testing.T) {
	verifyH, authH, participants, _ := setupVerifyTest(t)

	// Register a user
	p := registerUser(t, authH, "verify@example.com", "password123", "VerifyUser")

	// Generate and store a verification token
	token, err := auth.GenerateVerificationToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	err = participants.SetVerificationToken(t.Context(), p.ID, token, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("setting token: %v", err)
	}

	// Call verify endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token="+token, nil)
	rec := httptest.NewRecorder()
	verifyH.VerifyEmail(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]string
	testutil.DecodeResponse(t, rec, &resp)
	if resp["status"] != "verified" {
		t.Errorf("expected status 'verified', got %q", resp["status"])
	}
	if resp["participant_id"] != p.ID {
		t.Errorf("expected participant_id %q, got %q", p.ID, resp["participant_id"])
	}

	// Confirm the user is now verified
	verified, err := participants.IsEmailVerified(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("checking verification: %v", err)
	}
	if !verified {
		t.Error("expected email to be verified after successful verification")
	}
}

func TestEmailVerifyHandler_VerifyEmail_MissingToken(t *testing.T) {
	verifyH, _, _, _ := setupVerifyTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify-email", nil)
	rec := httptest.NewRecorder()
	verifyH.VerifyEmail(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestEmailVerifyHandler_VerifyEmail_InvalidToken(t *testing.T) {
	verifyH, _, _, _ := setupVerifyTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token=invalidtoken123", nil)
	rec := httptest.NewRecorder()
	verifyH.VerifyEmail(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestEmailVerifyHandler_VerifyEmail_ExpiredToken(t *testing.T) {
	verifyH, authH, participants, _ := setupVerifyTest(t)

	// Register a user
	p := registerUser(t, authH, "expired@example.com", "password123", "ExpiredUser")

	// Store a token that already expired
	token, err := auth.GenerateVerificationToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	err = participants.SetVerificationToken(t.Context(), p.ID, token, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("setting token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token="+token, nil)
	rec := httptest.NewRecorder()
	verifyH.VerifyEmail(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestEmailVerifyHandler_VerificationStatus_Unverified(t *testing.T) {
	verifyH, authH, _, cfg := setupVerifyTest(t)

	// Register a user (starts unverified)
	regReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "status@example.com",
		Password:    "password123",
		DisplayName: "StatusUser",
	})
	regRec := httptest.NewRecorder()
	authH.Register(regRec, regReq)
	testutil.AssertStatus(t, regRec, http.StatusCreated)

	var regResp map[string]any
	testutil.DecodeResponse(t, regRec, &regResp)
	accessToken := regResp["access_token"].(string)

	// Call verification-status with auth
	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/auth/verification-status", accessToken, nil)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(verifyH.VerificationStatus))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]bool
	testutil.DecodeResponse(t, rec, &resp)
	if resp["verified"] != false {
		t.Error("expected verified=false for new user")
	}
}

func TestEmailVerifyHandler_VerificationStatus_Verified(t *testing.T) {
	verifyH, authH, participants, cfg := setupVerifyTest(t)

	// Register a user
	p := registerUser(t, authH, "verified@example.com", "password123", "VerifiedUser")

	// Generate, store, and verify the token
	token, err := auth.GenerateVerificationToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	err = participants.SetVerificationToken(t.Context(), p.ID, token, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("setting token: %v", err)
	}
	_, err = participants.VerifyEmail(t.Context(), token)
	if err != nil {
		t.Fatalf("verifying email: %v", err)
	}

	// Generate access token for this user
	accessToken, err := auth.GenerateToken(cfg.JWT.Secret, time.Hour, p.ID, "human")
	if err != nil {
		t.Fatalf("generating auth token: %v", err)
	}

	// Call verification-status
	req := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/auth/verification-status", accessToken, nil)
	rec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(verifyH.VerificationStatus))
	protected.ServeHTTP(rec, req)

	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp map[string]bool
	testutil.DecodeResponse(t, rec, &resp)
	if resp["verified"] != true {
		t.Error("expected verified=true after email verification")
	}
}

// Register must NOT echo the verification token back to the caller.
// Tokens are delivered exclusively via the outbound email.
func TestRegister_DoesNotLeakVerificationToken(t *testing.T) {
	_, authH, _, _ := setupVerifyTest(t)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "newuser@example.com",
		Password:    "password123",
		DisplayName: "NewUser",
	})
	rec := httptest.NewRecorder()
	authH.Register(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var resp map[string]any
	testutil.DecodeResponse(t, rec, &resp)

	if _, ok := resp["verification_url"]; ok {
		t.Error("register response must not include verification_url (token leak)")
	}
	if _, ok := resp["verification_token"]; ok {
		t.Error("register response must not include verification_token (token leak)")
	}
}
