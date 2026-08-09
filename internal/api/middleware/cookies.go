package middleware

import "net/http"

// Auth-cookie names + paths.
//
// `lf_access` rides on every authenticated request to the API
// (Path=/). `lf_refresh` is scoped to /api/v1/auth/ so it is NOT
// attached to every request — only the auth-flow endpoints get to
// see the refresh token, limiting exposure if a downstream handler
// ever logs or echoes request headers.
const (
	AccessCookieName  = "lf_access"
	RefreshCookieName = "lf_refresh"

	accessCookiePath  = "/"
	refreshCookiePath = "/api/v1/auth/"

	// MaxAge values are deliberately *not* tied to the JWT TTL — the
	// JWT is validated server-side on every request, so even if the
	// cookie outlives the token the server rejects stale tokens. The
	// numbers here match the existing refresh-token DB TTL (7 days)
	// and the access-token TTL (15 minutes).
	accessCookieMaxAge  = 15 * 60       // 15 minutes
	refreshCookieMaxAge = 7 * 24 * 3600 // 7 days
)

// SetAuthCookies writes both access + refresh cookies on the response.
// `secure` should be true in production; pass false in development so
// plain-HTTP local doesn't silently strip the cookie.
//
// Cookie attributes:
//   - HttpOnly      — JS cannot read these; XSS no longer steals tokens
//   - Secure        — production only (HTTPS required)
//   - SameSite=Lax  — sent on top-level cross-site GET (needed for OAuth
//                     callback redirects from github.com), withheld on
//                     cross-site POST (the actual CSRF risk)
func SetAuthCookies(w http.ResponseWriter, accessToken, refreshToken string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessCookieName,
		Value:    accessToken,
		Path:     accessCookiePath,
		MaxAge:   accessCookieMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    refreshToken,
		Path:     refreshCookiePath,
		MaxAge:   refreshCookieMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// SetAccessCookie rotates only the access cookie. Used by /auth/refresh
// where the refresh credential is unchanged (the bcrypt-hashed refresh
// token in the DB is the source of truth; the cookie is just delivery).
func SetAccessCookie(w http.ResponseWriter, accessToken string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessCookieName,
		Value:    accessToken,
		Path:     accessCookiePath,
		MaxAge:   accessCookieMaxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookies expires both auth cookies. Use on logout. The exact
// Path values must match the originals or the browser will keep the
// old cookie around — see RFC 6265 §5.3.
func ClearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, c := range []*http.Cookie{
		{Name: AccessCookieName, Path: accessCookiePath},
		{Name: RefreshCookieName, Path: refreshCookiePath},
	} {
		c.MaxAge = -1
		c.HttpOnly = true
		c.Secure = secure
		c.SameSite = http.SameSiteLaxMode
		http.SetCookie(w, c)
	}
}

// ReadAccessCookie returns the value of the access cookie, or "" if
// not present. Callers should treat any non-empty return as a candidate
// token still requiring JWT signature validation.
func ReadAccessCookie(r *http.Request) string {
	c, err := r.Cookie(AccessCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// ReadRefreshCookie returns the value of the refresh cookie, or "".
func ReadRefreshCookie(r *http.Request) string {
	c, err := r.Cookie(RefreshCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}
