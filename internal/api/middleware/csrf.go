package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// CSRF returns middleware that rejects cross-origin mutating requests
// authenticated by ambient credentials (cookies). It is intentionally
// permissive in three cases that don't need protection:
//
//  1. Safe methods (GET, HEAD, OPTIONS) — by HTTP spec, these must not
//     have side effects. Cookie auth is fine.
//  2. Bearer-authenticated requests — SDKs, agents, mobile clients.
//     Bearer tokens are not ambient credentials; a CSRF attacker has no
//     way to make a victim's browser send them.
//  3. API-key authenticated requests — same logic. The `X-API-Key`
//     header is opt-in, not ambient.
//
// For the remaining case — cookie-auth (or unauth) mutating requests
// from a browser — we require the Origin (or Referer fallback) to
// match an entry in allowedOrigins. Mismatched or missing → 403.
//
// allowedOrigins should be the same set used by the CORS middleware;
// in practice both are fed from config.API.AllowedOrigins. A literal
// "*" entry is honored as a dev-mode wildcard (any origin allowed),
// which is why production deploys must list explicit origins.
func CSRF(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	wildcard := false
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
			continue
		}
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			if hasNonCookieCreds(r) {
				next.ServeHTTP(w, r)
				return
			}

			// No auth cookies present → no ambient credentials to
			// abuse → CSRF doesn't apply. This is the path that lets
			// SDK login(), Postman, server-to-server scripts, and
			// any other non-browser caller hit POST endpoints
			// without setting Origin. Browser-based CSRF requires
			// ambient cookies; without them the request is just an
			// anonymous one that the handler authenticates (or
			// rejects) on its own.
			//
			// Cross-site JSON POSTs are additionally defended by
			// CORS preflights — an attacker site cannot send
			// Content-Type: application/json to our endpoints
			// without an explicit allow-list entry in CORS.
			if !hasAuthCookie(r) {
				// No ambient cookie credentials. Non-browser callers (SDKs,
				// scripts, server-to-server) send no Origin and pass through —
				// they authenticate via the request body. But browsers DO
				// attach an Origin to cross-site POSTs, so when one is present
				// we still require it to be allowed. This blocks login-CSRF
				// (an attacker page auto-submitting POST /auth/login to log a
				// victim into the attacker's account) without breaking
				// non-browser clients.
				if origin := requestOrigin(r); origin != "" && !wildcard {
					if _, ok := allowed[origin]; !ok {
						http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
						return
					}
				}
				next.ServeHTTP(w, r)
				return
			}

			origin := requestOrigin(r)
			if origin == "" {
				http.Error(w, `{"error":"missing origin"}`, http.StatusForbidden)
				return
			}

			if wildcard {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// requestOrigin returns the request's browser origin, from the Origin header
// or derived from Referer. Empty string means no browser origin was sent
// (typical of non-browser clients).
func requestOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

// hasNonCookieCreds reports whether the request carries an auth
// credential that an attacker cannot forge cross-origin. Used by CSRF
// to skip the origin check when the request authenticates via Bearer
// or API key.
func hasNonCookieCreds(r *http.Request) bool {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") && len(h) > len("Bearer ") {
		return true
	}
	if r.Header.Get("X-API-Key") != "" {
		return true
	}
	return false
}

// hasAuthCookie reports whether the request carries one of our session
// cookies. CSRF is enforced only when ambient credentials are present;
// without a session cookie there's nothing the middleware can defend.
func hasAuthCookie(r *http.Request) bool {
	if _, err := r.Cookie(AccessCookieName); err == nil {
		return true
	}
	if _, err := r.Cookie(RefreshCookieName); err == nil {
		return true
	}
	return false
}
