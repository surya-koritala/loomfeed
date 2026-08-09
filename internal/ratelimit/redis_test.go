package ratelimit

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/RoamXAI/loomfeed/internal/cache"
)

func newTestLimiter(t *testing.T, name string, max int, window time.Duration) (*RedisLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rc := cache.NewRedisCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	return NewRedisLimiter(rc, name, max, window), mr
}

func TestRedisLimiter_AllowsUpToMaxThenBlocks(t *testing.T) {
	rl, _ := newTestLimiter(t, "post", 3, time.Minute)

	for i := 1; i <= 3; i++ {
		if !rl.Allow("agent-1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if rl.Allow("agent-1") {
		t.Fatal("4th request should be blocked")
	}
}

func TestRedisLimiter_KeysAreIndependent(t *testing.T) {
	rl, _ := newTestLimiter(t, "post", 1, time.Minute)

	if !rl.Allow("agent-1") {
		t.Fatal("agent-1 first request should be allowed")
	}
	if rl.Allow("agent-1") {
		t.Fatal("agent-1 second request should be blocked")
	}
	// A different participant has its own bucket.
	if !rl.Allow("agent-2") {
		t.Fatal("agent-2 should be unaffected by agent-1's usage")
	}
}

func TestRedisLimiter_NameNamespacesTheBucket(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rc := cache.NewRedisCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	post := NewRedisLimiter(rc, "post", 1, time.Minute)
	vote := NewRedisLimiter(rc, "vote", 1, time.Minute)

	if !post.Allow("agent-1") {
		t.Fatal("post action should be allowed")
	}
	// Same id, different action name → different counter, still allowed.
	if !vote.Allow("agent-1") {
		t.Fatal("vote action must not share post's counter")
	}
}

func TestRedisLimiter_Remaining(t *testing.T) {
	rl, _ := newTestLimiter(t, "post", 5, time.Minute)

	if got := rl.Remaining("agent-1"); got != 5 {
		t.Fatalf("fresh budget: want 5, got %d", got)
	}
	rl.Allow("agent-1")
	rl.Allow("agent-1")
	if got := rl.Remaining("agent-1"); got != 3 {
		t.Fatalf("after 2 calls: want 3, got %d", got)
	}
}

func TestRedisLimiter_WindowExpiryResets(t *testing.T) {
	rl, mr := newTestLimiter(t, "post", 1, time.Minute)

	if !rl.Allow("agent-1") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("agent-1") {
		t.Fatal("second request in window should be blocked")
	}
	// Advance past the window — the counter key TTLs out and budget refills.
	mr.FastForward(61 * time.Second)
	if !rl.Allow("agent-1") {
		t.Fatal("request after window expiry should be allowed again")
	}
}

func TestRedisLimiter_FailsOpenWhenRedisUnavailable(t *testing.T) {
	// A nil *cache.RedisCache stands in for "Redis not configured / down".
	// Its window ops return an error, and the limiter must fail open.
	rl := NewRedisLimiter(cache.NewRedisCache(nil), "post", 1, time.Minute)

	for i := 0; i < 10; i++ {
		if !rl.Allow("agent-1") {
			t.Fatalf("request %d must be allowed when Redis is unavailable (fail open)", i)
		}
	}
	if got := rl.Remaining("agent-1"); got != 1 {
		t.Fatalf("Remaining should report full budget on error, want 1, got %d", got)
	}
}
