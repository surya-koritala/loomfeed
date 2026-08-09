package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogger_RedactsTokenParam(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := Logger(inner)

	// Request with token in query string
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream?token=eyJhbGciOiJIUzI1NiJ9.secret", nil)
	rec := httptest.NewRecorder()
	logger.ServeHTTP(rec, req)

	logged := buf.String()
	if strings.Contains(logged, "eyJhbGciOiJIUzI1NiJ9") {
		t.Errorf("JWT token was NOT redacted from logs: %s", logged)
	}
	if !strings.Contains(logged, "token=***") {
		t.Errorf("expected 'token=***' in logged path, got: %s", logged)
	}
}

func TestLogger_PreservesNonSensitiveQuery(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := Logger(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=hello&limit=10", nil)
	rec := httptest.NewRecorder()
	logger.ServeHTTP(rec, req)

	logged := buf.String()
	if !strings.Contains(logged, "q=hello") {
		t.Errorf("expected normal query params to be preserved in log, got: %s", logged)
	}
}

func TestLogger_NoQueryString(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	slog.SetDefault(slog.New(handler))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := Logger(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/123", nil)
	rec := httptest.NewRecorder()
	logger.ServeHTTP(rec, req)

	logged := buf.String()
	if !strings.Contains(logged, "/api/v1/posts/123") {
		t.Errorf("expected path in log, got: %s", logged)
	}
	if strings.Contains(logged, "?") {
		t.Errorf("unexpected query string in log for path-only request: %s", logged)
	}
}

// TestLogger_RedactsOAuthParams covers the audit-driven extension of
// the redaction set. The OAuth callback round-trip puts `code` and
// `state` on a URL the access logger sees; we don't want either in
// log aggregators.
func TestLogger_RedactsOAuthParams(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		secret  string
		paramKV string // expected redacted form, e.g. "code=***"
	}{
		{
			name:    "code is redacted",
			path:    "/api/v1/auth/github/callback?code=ghu_abcdef0123456789&state=somesate",
			secret:  "ghu_abcdef0123456789",
			paramKV: "code=***",
		},
		{
			name:    "state is redacted",
			path:    "/api/v1/auth/github/callback?code=ghu_abcdef0123456789&state=zZzZ-csrf-state-zZzZ",
			secret:  "zZzZ-csrf-state-zZzZ",
			paramKV: "state=***",
		},
		{
			name:    "access_token is redacted",
			path:    "/api/v1/auth/google?access_token=ya29.SECRETPART",
			secret:  "ya29.SECRETPART",
			paramKV: "access_token=***",
		},
		{
			name:    "id_token is redacted",
			path:    "/api/v1/auth/google?id_token=eyJ.HEADER.PAYLOAD",
			secret:  "eyJ.HEADER.PAYLOAD",
			paramKV: "id_token=***",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
			Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, nil))

			logged := buf.String()
			if strings.Contains(logged, tc.secret) {
				t.Errorf("secret leaked in log: %s", logged)
			}
			if !strings.Contains(logged, tc.paramKV) {
				t.Errorf("expected %q in log, got: %s", tc.paramKV, logged)
			}
		})
	}
}

func TestLogger_PreservesUnknownParams(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet,
			"/api/v1/feed?cursor=abc&limit=10&token=should_be_redacted&q=test", nil))

	logged := buf.String()
	if !strings.Contains(logged, "cursor=abc") || !strings.Contains(logged, "limit=10") || !strings.Contains(logged, "q=test") {
		t.Errorf("non-sensitive params should be preserved, got: %s", logged)
	}
	if strings.Contains(logged, "should_be_redacted") {
		t.Errorf("token was not redacted in mixed-param query: %s", logged)
	}
}
