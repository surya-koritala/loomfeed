package routes

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
)

func TestBYOKAvailabilityAndUnavailableRoutes(t *testing.T) {
	pool := database.TestPool(t)
	const jwtSecret = "byok-route-test-secret"
	token, err := auth.GenerateToken(jwtSecret, time.Hour,
		"11111111-1111-4111-8111-111111111111", string(models.ParticipantHuman))
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	request := func(mux *http.ServeMux, method, target, body string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	for _, tt := range []struct {
		name string
		byok config.BYOKConfig
	}{
		{name: "missing key"},
		{name: "malformed key", byok: config.BYOKConfig{Enabled: true, KEK: "not-base64"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			cfg := &config.Config{JWT: config.JWTConfig{Secret: jwtSecret}, BYOK: tt.byok}
			Register(mux, pool, cfg, registerOptions{disableBackgroundWorkers: true})

			publicConfig := request(mux, http.MethodGet, "/api/v1/config", "", false)
			if publicConfig.Code != http.StatusOK ||
				!strings.Contains(publicConfig.Body.String(), `"byok_enabled":false`) {
				t.Fatalf("config status=%d body=%s", publicConfig.Code, publicConfig.Body.String())
			}

			create := request(mux, http.MethodPost, "/api/v1/byok-agents",
				`{"display_name":"Test","provider":"openai","model":"test","api_key":"secret"}`, true)
			if create.Code != http.StatusServiceUnavailable ||
				!strings.Contains(create.Body.String(), "BYOK agents are not available") {
				t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
			}

			summon := request(mux, http.MethodPost, "/api/v1/posts/post-id/summon",
				`{"byok_agent_id":"agent-id"}`, true)
			if summon.Code != http.StatusServiceUnavailable ||
				!strings.Contains(summon.Body.String(), "BYOK agents are not available") {
				t.Fatalf("summon status=%d body=%s", summon.Code, summon.Body.String())
			}

			deleteAgent := request(mux, http.MethodDelete, "/api/v1/byok-agents/agent-id", "", true)
			if deleteAgent.Code != http.StatusServiceUnavailable ||
				!strings.Contains(deleteAgent.Body.String(), "BYOK agents are not available") {
				t.Fatalf("delete status=%d body=%s", deleteAgent.Code, deleteAgent.Body.String())
			}
		})
	}

	t.Run("valid key is public", func(t *testing.T) {
		mux := http.NewServeMux()
		kek := base64.StdEncoding.EncodeToString(make([]byte, 32))
		cfg := &config.Config{
			JWT:  config.JWTConfig{Secret: jwtSecret},
			BYOK: config.BYOKConfig{Enabled: true, KEK: kek},
		}
		Register(mux, pool, cfg, registerOptions{disableBackgroundWorkers: true})
		publicConfig := request(mux, http.MethodGet, "/api/v1/config", "", false)
		if publicConfig.Code != http.StatusOK ||
			!strings.Contains(publicConfig.Body.String(), `"byok_enabled":true`) {
			t.Fatalf("config status=%d body=%s", publicConfig.Code, publicConfig.Body.String())
		}
	})
}
