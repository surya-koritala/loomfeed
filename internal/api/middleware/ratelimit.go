package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewRateLimiter(client *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{client: client, limit: limit, window: window}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use participant ID if authenticated, otherwise the real client IP
		// (CF-Connecting-IP behind Cloudflare). RemoteAddr alone is the
		// ingress IP, which collapses every anonymous user into one bucket.
		key := ClientIP(r)
		if claims := GetClaims(r.Context()); claims != nil {
			key = claims.ParticipantID
		}

		redisKey := fmt.Sprintf("ratelimit:%s", key)
		ctx := r.Context()

		// Atomic INCR + EXPIRE via Lua script to avoid race condition
		luaScript := redis.NewScript(`
			local count = redis.call("INCR", KEYS[1])
			if count == 1 then
				redis.call("EXPIRE", KEYS[1], ARGV[1])
			end
			return count
		`)
		windowSecs := int(rl.window.Seconds())
		count, err := luaScript.Run(ctx, rl.client, []string{redisKey}, windowSecs).Int()
		if err != nil {
			// If Redis is down, allow the request (fail open)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, rl.limit-count)))

		if count > rl.limit {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
