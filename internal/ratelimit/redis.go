package ratelimit

import (
	"context"
	"time"
)

// redisBackend is the slice of *cache.RedisCache that a RedisLimiter
// needs. Declaring it here (rather than importing the cache package's
// concrete type) keeps the dependency one-directional and makes the
// limiter trivially testable with a fake.
type redisBackend interface {
	// IncrWindow increments the fixed-window counter at key, sets a TTL
	// of window on the first increment, and returns the new count.
	IncrWindow(ctx context.Context, key string, window time.Duration) (int64, error)
	// PeekCount returns the current counter value without mutating it.
	PeekCount(ctx context.Context, key string) (int64, error)
}

// RedisLimiter enforces a maximum number of actions per fixed window,
// keyed by participant id, using a shared Redis counter. Because the
// counter lives in Redis, the limit holds across every API replica —
// unlike the in-memory limiter, whose per-process counters let the
// effective limit scale with the replica count.
//
// It FAILS OPEN: if Redis is unreachable, Allow returns true. A rate
// limiter that hard-fails takes the whole API down with Redis, which is
// a worse outcome than briefly unmetered traffic.
type RedisLimiter struct {
	backend redisBackend
	name    string // namespaces the key, e.g. "post" → "rl:post:<id>"
	max     int
	window  time.Duration
}

// NewRedisLimiter builds a cross-replica limiter. name must be unique
// per logical action (post/comment/vote/…) so their counters don't
// collide in the shared keyspace.
func NewRedisLimiter(backend redisBackend, name string, max int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{backend: backend, name: name, max: max, window: window}
}

func (rl *RedisLimiter) key(id string) string {
	return "rl:" + rl.name + ":" + id
}

// Allow increments the caller's counter and reports whether they remain
// within budget. Any Redis error fails open (returns true).
func (rl *RedisLimiter) Allow(id string) bool {
	count, err := rl.backend.IncrWindow(context.Background(), rl.key(id), rl.window)
	if err != nil {
		return true // fail open — availability over strict enforcement
	}
	return count <= int64(rl.max)
}

// Remaining is best-effort; on any error it reports the full budget so
// the header never under-reports due to a transient Redis blip.
func (rl *RedisLimiter) Remaining(id string) int {
	count, err := rl.backend.PeekCount(context.Background(), rl.key(id))
	if err != nil {
		return rl.max
	}
	remaining := rl.max - int(count)
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}
