package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func csrfOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRF_SafeMethodsAlwaysPass(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(m, "/api/v1/posts", nil)
		// No Origin header — would normally fail.
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s should pass without Origin, got %d", m, rec.Code)
		}
	}
}

func TestCSRF_BearerSkipsOriginCheck(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGc")
	// Note: NO Origin header.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Bearer-auth POST should bypass CSRF, got %d", rec.Code)
	}
}

func TestCSRF_APIKeySkipsOriginCheck(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	req.Header.Set("X-API-Key", "ak_abc")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("API-key POST should bypass CSRF, got %d", rec.Code)
	}
}

// addSessionCookie attaches an lf_access cookie so CSRF actually fires.
// Without it, the middleware skips (no ambient creds to abuse).
func addSessionCookie(r *http.Request) {
	r.AddCookie(&http.Cookie{Name: AccessCookieName, Value: "jwt-blob"})
}

func TestCSRF_NoSessionCookieSkips(t *testing.T) {
	// SDKs and server-to-server scripts hit auth endpoints without
	// any session cookie and without setting Origin. They must not
	// be blocked — there's no ambient credential to abuse.
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	// No Origin, no Bearer, no API key, no session cookie.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("no-session POST should pass CSRF, got %d", rec.Code)
	}
}

func TestCSRF_OriginMatchPasses(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	addSessionCookie(req)
	req.Header.Set("Origin", "https://www.loomfeed.com")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("matching Origin should pass, got %d", rec.Code)
	}
}

func TestCSRF_OriginMismatchBlocked(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	addSessionCookie(req)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("mismatched Origin should 403, got %d", rec.Code)
	}
}

func TestCSRF_MissingOriginWithSessionBlocked(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	addSessionCookie(req)
	// No Origin, no Referer, BUT session cookie present — must 403.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("session cookie + missing Origin should 403, got %d", rec.Code)
	}
}

func TestCSRF_RefererFallback(t *testing.T) {
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	addSessionCookie(req)
	req.Header.Set("Referer", "https://www.loomfeed.com/feed?tab=hot")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Referer should be used as Origin fallback, got %d", rec.Code)
	}
}

func TestCSRF_WildcardAllowsAny(t *testing.T) {
	csrf := CSRF([]string{"*"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	addSessionCookie(req)
	req.Header.Set("Origin", "https://random.example")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("wildcard origin allow should pass any Origin, got %d", rec.Code)
	}
}

func TestCSRF_RefreshCookieAlsoTriggers(t *testing.T) {
	// Either cookie should be enough to trigger CSRF enforcement —
	// the refresh cookie is enough ambient credential for the
	// /auth/refresh endpoint to be CSRF-abusable.
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: "refresh-blob"})
	// No Origin — should 403.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("refresh cookie alone + missing Origin should 403, got %d", rec.Code)
	}
}

func TestCSRF_EmptyAuthorizationDoesNotSkip(t *testing.T) {
	// An empty `Authorization` header (or one that just says "Bearer "
	// with no token) should NOT be treated as a valid Bearer credential.
	// Combined with a session cookie + no Origin → 403.
	csrf := CSRF([]string{"https://www.loomfeed.com"})
	srv := csrf(csrfOKHandler())

	cases := []string{"", "Bearer ", "Basic abc"}
	for _, h := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
		addSessionCookie(req)
		if h != "" {
			req.Header.Set("Authorization", h)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("Authorization=%q with session + no Origin should 403, got %d", h, rec.Code)
		}
	}
}
