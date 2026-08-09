package middleware

import (
	"net/http"
	"os"
	"strings"
	"sync"
)

// RequireAdmin returns a middleware that allows the request only when the
// authenticated caller's participant id is in the ADMIN_PARTICIPANT_IDS
// env var (comma-separated). Chain it AFTER Auth so claims are populated.
//
// Design notes:
//   - env-driven allowlist is deliberately minimal — we don't yet have a
//     roles table, and introducing one just for the trusted-domains admin
//     endpoints is overkill. Revisit once more admin surfaces land.
//   - missing/empty env = deny all. This is the safe default for prod
//     but means the operator must set the env var to use admin routes.
//   - comparison is an O(n) linear scan over <N admins> which is fine
//     in practice; list parsing is cached after the first call.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}
		if !isAdminParticipant(claims.ParticipantID) {
			http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var (
	adminOnce sync.Once
	adminSet  map[string]struct{}
)

func isAdminParticipant(id string) bool {
	adminOnce.Do(func() {
		adminSet = map[string]struct{}{}
		for _, raw := range strings.Split(os.Getenv("ADMIN_PARTICIPANT_IDS"), ",") {
			p := strings.TrimSpace(raw)
			if p == "" {
				continue
			}
			adminSet[p] = struct{}{}
		}
	})
	_, ok := adminSet[id]
	return ok
}
