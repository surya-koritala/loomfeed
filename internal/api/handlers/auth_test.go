package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

func setupAuthTest(t *testing.T) (*handlers.AuthHandler, *config.Config) {
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
	return handlers.NewAuthHandler(participants, refreshTokens, pool, cfg), cfg
}

func TestAuthHandler_Register_Success(t *testing.T) {
	handler, _ := setupAuthTest(t)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "alice@example.com",
		Password:    "supersecret",
		DisplayName: "Alice",
	})
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	testutil.AssertStatus(t, rec, http.StatusCreated)

	var resp models.AuthResponse
	testutil.DecodeResponse(t, rec, &resp)

	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Participant == nil {
		t.Error("expected participant in response")
	}
}

func TestAuthHandler_Register_MissingFields(t *testing.T) {
	handler, _ := setupAuthTest(t)

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "",
		Password:    "somepass",
		DisplayName: "Bob",
	})
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestAuthHandler_Register_DuplicateEmail(t *testing.T) {
	handler, _ := setupAuthTest(t)

	body := models.RegisterRequest{
		Email:       "dupe@example.com",
		Password:    "password123",
		DisplayName: "Dupe User",
	}

	// First registration should succeed.
	req1 := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", body)
	rec1 := httptest.NewRecorder()
	handler.Register(rec1, req1)
	testutil.AssertStatus(t, rec1, http.StatusCreated)

	// Second registration with same email should return 409.
	req2 := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", body)
	rec2 := httptest.NewRecorder()
	handler.Register(rec2, req2)
	testutil.AssertStatus(t, rec2, http.StatusConflict)
}

func TestAuthHandler_Login_Success(t *testing.T) {
	handler, _ := setupAuthTest(t)

	// Register first.
	regReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "login@example.com",
		Password:    "mypassword",
		DisplayName: "Login User",
	})
	regRec := httptest.NewRecorder()
	handler.Register(regRec, regReq)
	testutil.AssertStatus(t, regRec, http.StatusCreated)

	// Now login.
	loginReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/login", models.LoginRequest{
		Email:    "login@example.com",
		Password: "mypassword",
	})
	loginRec := httptest.NewRecorder()
	handler.Login(loginRec, loginReq)
	testutil.AssertStatus(t, loginRec, http.StatusOK)

	var resp models.AuthResponse
	testutil.DecodeResponse(t, loginRec, &resp)

	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	handler, _ := setupAuthTest(t)

	// Register first.
	regReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "wrongpass@example.com",
		Password:    "correctpassword",
		DisplayName: "Wrong Pass User",
	})
	regRec := httptest.NewRecorder()
	handler.Register(regRec, regReq)
	testutil.AssertStatus(t, regRec, http.StatusCreated)

	// Login with wrong password.
	loginReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/login", models.LoginRequest{
		Email:    "wrongpass@example.com",
		Password: "wrongpassword",
	})
	loginRec := httptest.NewRecorder()
	handler.Login(loginRec, loginReq)
	testutil.AssertStatus(t, loginRec, http.StatusUnauthorized)
}

func TestAuthHandler_Me_Success(t *testing.T) {
	handler, cfg := setupAuthTest(t)

	// Register to get a token.
	regReq := testutil.JSONRequest(t, http.MethodPost, "/api/v1/auth/register", models.RegisterRequest{
		Email:       "me@example.com",
		Password:    "mepassword",
		DisplayName: "Me User",
	})
	regRec := httptest.NewRecorder()
	handler.Register(regRec, regReq)
	testutil.AssertStatus(t, regRec, http.StatusCreated)

	var regResp models.AuthResponse
	testutil.DecodeResponse(t, regRec, &regResp)

	// Call /me with the auth middleware wrapping the handler.
	meReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/auth/me", regResp.Token, nil)
	meRec := httptest.NewRecorder()

	protected := middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Me))
	protected.ServeHTTP(meRec, meReq)

	testutil.AssertStatus(t, meRec, http.StatusOK)

	var participant models.Participant
	testutil.DecodeResponse(t, meRec, &participant)

	if participant.ID == "" {
		t.Error("expected participant ID in response")
	}
	if participant.DisplayName != "Me User" {
		t.Errorf("expected display_name 'Me User', got %q", participant.DisplayName)
	}
}
