package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/database"
)

// TokenBlacklist is the Redis cache used to check if a JWT has been revoked
// (e.g., after password change). Set during server initialization. When nil,
// the blacklist check is skipped (graceful degradation).
var TokenBlacklist *cache.RedisCache

type contextKey string

const ClaimsKey contextKey = "claims"

// sensitiveQueryParams is the set of query parameter names whose VALUES
// must never appear in access logs. Anything matching is replaced with
// "***" before logging.
//
// `token` — JWTs that legacy SSE clients pass via query string.
// `code` and `state` — OAuth round-trip values; the code is single-use
// but logging it widens the window for replay; state is browser-bound
// CSRF entropy that should not be inspectable from log aggregators.
// `access_token` and `id_token` — defense in depth for any future flow.
var sensitiveQueryParams = map[string]struct{}{
	"token":        {},
	"access_token": {},
	"id_token":     {},
	"code":         {},
	"state":        {},
}

// redactQuery replaces values of sensitive params with "***" while
// preserving other params for debugging. Returns the redacted query
// string (without leading "?").
func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	pairs := strings.Split(raw, "&")
	for i, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			continue
		}
		name := p[:eq]
		if _, sensitive := sensitiveQueryParams[name]; sensitive {
			pairs[i] = name + "=***"
		}
	}
	return strings.Join(pairs, "&")
}

// Logger logs each request with method, path, status, and duration.
// Sensitive query parameters (token, code, state, access_token, id_token)
// are redacted from the logged path so OAuth codes, CSRF state, and JWTs
// do not end up in access logs.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		logPath := r.URL.Path
		if q := redactQuery(r.URL.RawQuery); q != "" {
			logPath += "?" + q
		}

		slog.Info("request",
			"method", r.Method,
			"path", logPath,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}

// CORS adds cross-origin headers for the web frontend.
// allowedOrigins is a list of permitted origins; use ["*"] to allow all (dev only).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds common security-related HTTP response headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// extractJWT returns the JWT to validate for this request. Preference
// order: Authorization: Bearer (SDKs, agents) → lf_access cookie
// (browser). The `format-ok` return distinguishes "no header at all"
// (caller may have a cookie path) from "header present but malformed"
// (caller should 401 with "invalid authorization format" for
// backwards compat with existing SDK error messages).
func extractJWT(r *http.Request) (token string, formatOK bool, fromHeader bool) {
	header := r.Header.Get("Authorization")
	if header != "" {
		t := strings.TrimPrefix(header, "Bearer ")
		if t == header {
			return "", false, true // header present but not Bearer
		}
		return t, true, true
	}
	return ReadAccessCookie(r), true, false
}

// OptionalAuth tries to validate JWT but doesn't reject — sets claims if valid, passes through if not.
// Reads from `Authorization: Bearer ...` first, falling back to the
// `lf_access` cookie. Cookie path is gated to skip API-key-looking
// tokens (those starting with `ak_`) so the API-key middleware can
// claim them first.
func OptionalAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetClaims(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			token, ok, _ := extractJWT(r)
			if !ok || token == "" || strings.HasPrefix(token, "ak_") {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := auth.ValidateToken(secret, token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			// Check token blacklist before accepting
			if TokenBlacklist != nil {
				blacklistKey := "token_blacklist:" + token
				if cached, _ := TokenBlacklist.Get(r.Context(), blacklistKey); cached != nil {
					next.ServeHTTP(w, r)
					return
				}
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = database.WithUserID(ctx, claims.ParticipantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Auth validates JWT tokens from the Authorization header, falling back
// to the `lf_access` cookie. If claims are already set in context (e.g.,
// by APIKeyAuth), it passes through.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If claims already set by a prior auth middleware (e.g., API key), skip JWT check
			if GetClaims(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}

			token, formatOK, fromHeader := extractJWT(r)
			if fromHeader && !formatOK {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}
			if token == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateToken(secret, token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Check token blacklist (revoked tokens, e.g., after password change)
			if TokenBlacklist != nil {
				blacklistKey := "token_blacklist:" + token
				if cached, _ := TokenBlacklist.Get(r.Context(), blacklistKey); cached != nil {
					http.Error(w, `{"error":"token revoked"}`, http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = database.WithUserID(ctx, claims.ParticipantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaims extracts auth claims from the request context.
func GetClaims(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(ClaimsKey).(*auth.Claims)
	return claims
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush forwards to the inner ResponseWriter when it implements
// http.Flusher. Without this, *statusWriter satisfies only
// http.ResponseWriter — Go's interface embedding does NOT promote
// methods from the embedded interface's dynamic type to the
// wrapping struct's compile-time method set. The mcp-go SSE handler
// type-asserts w.(http.Flusher) and replies "Streaming unsupported"
// when it fails, which is what blew up MCP across the platform.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets net/http's response controller (and downstream
// middlewares like the gzip wrapper) find the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
