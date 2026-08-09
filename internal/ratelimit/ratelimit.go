// Package ratelimit provides in-memory per-participant rate limiting using
// a sliding window counter approach. It is designed for use in HTTP handlers
// to prevent spam floods from individual participants.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is the behaviour the HTTP handlers depend on. Both the
// in-memory *RateLimiter (single-process, sliding window) and the
// *RedisLimiter (cross-replica, fixed window) satisfy it, so routes
// can pick a backend at startup without the handlers caring which.
type Limiter interface {
	// Allow reports whether the action for id is permitted, recording
	// it against the window when so.
	Allow(id string) bool
	// Remaining is a best-effort count of actions left in the current
	// window for id (used only to populate X-RateLimit-Remaining).
	Remaining(id string) int
}

// entry tracks the timestamps of actions within the sliding window.
type entry struct {
	timestamps []time.Time
}

// RateLimiter enforces a maximum number of actions per time window
// on a per-participant basis using an in-memory sliding window.
type RateLimiter struct {
	mu     sync.Mutex
	limits map[string]*entry
	max    int
	window time.Duration
}

// New creates a new RateLimiter that allows max actions within the given window.
func New(max int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limits: make(map[string]*entry),
		max:    max,
		window: window,
	}
	// Start a background goroutine to periodically clean up stale entries.
	go rl.cleanup()
	return rl
}

// Allow checks whether the participant identified by id is allowed to perform
// an action. Returns true if the action is allowed, false if rate limited.
func (rl *RateLimiter) Allow(id string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	e, ok := rl.limits[id]
	if !ok {
		e = &entry{}
		rl.limits[id] = e
	}

	// Prune timestamps older than the window.
	pruned := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	e.timestamps = pruned

	if len(e.timestamps) >= rl.max {
		return false
	}

	e.timestamps = append(e.timestamps, now)
	return true
}

// Remaining returns the number of remaining allowed actions for the given participant.
func (rl *RateLimiter) Remaining(id string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	e, ok := rl.limits[id]
	if !ok {
		return rl.max
	}

	count := 0
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			count++
		}
	}

	remaining := rl.max - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// Reset clears the rate limit state for a specific participant.
func (rl *RateLimiter) Reset(id string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limits, id)
}

// cleanup periodically removes stale entries to prevent memory leaks.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for id, e := range rl.limits {
			// Remove entries with no recent timestamps.
			hasRecent := false
			for _, ts := range e.timestamps {
				if ts.After(cutoff) {
					hasRecent = true
					break
				}
			}
			if !hasRecent {
				delete(rl.limits, id)
			}
		}
		rl.mu.Unlock()
	}
}
