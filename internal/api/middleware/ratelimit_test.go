package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRateLimiter(t *testing.T, mr *miniredis.Miniredis, limit int) *RateLimiter {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRateLimiter(client, limit, time.Minute)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rl := newTestRateLimiter(t, mr, 5)
	handler := rl.Middleware(okHandler())

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rl := newTestRateLimiter(t, mr, 5)
	handler := rl.Middleware(okHandler())

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:5678"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// 6th request should be blocked
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5678"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on 6th request, got %d", rr.Code)
	}
}

func TestRateLimiter_SetsHeaders(t *testing.T) {
	mr := miniredis.RunT(t)
	rl := newTestRateLimiter(t, mr, 5)
	handler := rl.Middleware(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	limitHeader := rr.Header().Get("X-RateLimit-Limit")
	if limitHeader != "5" {
		t.Errorf("X-RateLimit-Limit: expected \"5\", got %q", limitHeader)
	}

	remainingHeader := rr.Header().Get("X-RateLimit-Remaining")
	remaining, err := strconv.Atoi(remainingHeader)
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining is not an integer: %q", remainingHeader)
	}
	if remaining != 4 {
		t.Errorf("X-RateLimit-Remaining: expected 4, got %d", remaining)
	}
}

func TestRateLimiter_FailsOpen(t *testing.T) {
	mr := miniredis.RunT(t)
	rl := newTestRateLimiter(t, mr, 5)
	handler := rl.Middleware(okHandler())

	// Stop miniredis to simulate Redis being down
	mr.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1111"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 when Redis is down (fail open), got %d", rr.Code)
	}
}
