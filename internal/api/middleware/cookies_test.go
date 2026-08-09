package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetAuthCookies_SetsBothWithExpectedAttrs(t *testing.T) {
	rec := httptest.NewRecorder()
	SetAuthCookies(rec, "access-value", "refresh-value", true)

	cookies := rec.Result().Cookies()
	var access, refresh *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case AccessCookieName:
			access = c
		case RefreshCookieName:
			refresh = c
		}
	}
	if access == nil {
		t.Fatalf("access cookie %q not set", AccessCookieName)
	}
	if refresh == nil {
		t.Fatalf("refresh cookie %q not set", RefreshCookieName)
	}

	if access.Value != "access-value" {
		t.Errorf("access cookie value: got %q, want %q", access.Value, "access-value")
	}
	if refresh.Value != "refresh-value" {
		t.Errorf("refresh cookie value: got %q, want %q", refresh.Value, "refresh-value")
	}

	if !access.HttpOnly {
		t.Error("access cookie should be HttpOnly")
	}
	if !refresh.HttpOnly {
		t.Error("refresh cookie should be HttpOnly")
	}
	if !access.Secure {
		t.Error("access cookie should be Secure when secure=true")
	}
	if access.SameSite != http.SameSiteLaxMode {
		t.Errorf("access SameSite: got %v, want Lax", access.SameSite)
	}
	if access.Path != "/" {
		t.Errorf("access path: got %q, want %q", access.Path, "/")
	}
	if refresh.Path != "/api/v1/auth/" {
		t.Errorf("refresh path: got %q, want %q", refresh.Path, "/api/v1/auth/")
	}
}

func TestSetAuthCookies_InsecureInDev(t *testing.T) {
	rec := httptest.NewRecorder()
	SetAuthCookies(rec, "a", "r", false)
	for _, c := range rec.Result().Cookies() {
		if c.Secure {
			t.Errorf("cookie %q should NOT be Secure when secure=false (dev)", c.Name)
		}
	}
}

func TestSetAccessCookie_OnlySetsAccess(t *testing.T) {
	rec := httptest.NewRecorder()
	SetAccessCookie(rec, "new-access", true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie set, got %d", len(cookies))
	}
	if cookies[0].Name != AccessCookieName {
		t.Errorf("got cookie %q, want %q", cookies[0].Name, AccessCookieName)
	}
}

func TestClearAuthCookies_ExpiresBoth(t *testing.T) {
	rec := httptest.NewRecorder()
	ClearAuthCookies(rec, true)
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 clear-cookies, got %d", len(cookies))
	}
	for _, c := range cookies {
		if c.MaxAge >= 0 {
			t.Errorf("cookie %q should be expired (MaxAge < 0), got %d", c.Name, c.MaxAge)
		}
	}
}

func TestReadAccessCookie_RoundTrip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: AccessCookieName, Value: "jwt-blob"})
	if got := ReadAccessCookie(req); got != "jwt-blob" {
		t.Errorf("ReadAccessCookie: got %q, want %q", got, "jwt-blob")
	}
}

func TestReadAccessCookie_AbsentReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := ReadAccessCookie(req); got != "" {
		t.Errorf("ReadAccessCookie with no cookie: got %q, want empty", got)
	}
}
