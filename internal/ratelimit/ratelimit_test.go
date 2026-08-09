package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := New(5, time.Minute)

	for i := 0; i < 5; i++ {
		if !rl.Allow("user1") {
			t.Errorf("expected Allow to return true on attempt %d", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := New(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.Allow("user1") {
			t.Fatalf("expected Allow to return true on attempt %d", i+1)
		}
	}

	// 4th attempt should be blocked.
	if rl.Allow("user1") {
		t.Error("expected Allow to return false when over limit")
	}
}

func TestRateLimiter_SeparateParticipants(t *testing.T) {
	rl := New(2, time.Minute)

	// Exhaust user1's limit.
	rl.Allow("user1")
	rl.Allow("user1")

	// user2 should still be allowed.
	if !rl.Allow("user2") {
		t.Error("expected user2 to be allowed independently of user1")
	}

	// user1 should now be blocked.
	if rl.Allow("user1") {
		t.Error("expected user1 to be blocked after exhausting limit")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	// Use a very short window for testing.
	rl := New(2, 50*time.Millisecond)

	rl.Allow("user1")
	rl.Allow("user1")

	// Should be blocked.
	if rl.Allow("user1") {
		t.Error("expected Allow to return false when limit reached")
	}

	// Wait for window to expire.
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again.
	if !rl.Allow("user1") {
		t.Error("expected Allow to return true after window expires")
	}
}

func TestRateLimiter_Remaining(t *testing.T) {
	rl := New(5, time.Minute)

	if rem := rl.Remaining("user1"); rem != 5 {
		t.Errorf("expected 5 remaining, got %d", rem)
	}

	rl.Allow("user1")
	rl.Allow("user1")

	if rem := rl.Remaining("user1"); rem != 3 {
		t.Errorf("expected 3 remaining, got %d", rem)
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := New(2, time.Minute)

	rl.Allow("user1")
	rl.Allow("user1")

	if rl.Allow("user1") {
		t.Error("expected user1 to be blocked")
	}

	rl.Reset("user1")

	if !rl.Allow("user1") {
		t.Error("expected user1 to be allowed after reset")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := New(100, time.Minute)
	var wg sync.WaitGroup

	allowed := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.Allow("user1")
		}()
	}

	wg.Wait()
	close(allowed)

	trueCount := 0
	for a := range allowed {
		if a {
			trueCount++
		}
	}

	if trueCount != 100 {
		t.Errorf("expected exactly 100 allowed, got %d", trueCount)
	}
}

func TestRateLimiter_SlidingWindow(t *testing.T) {
	rl := New(3, 100*time.Millisecond)

	// Use 2 of 3 slots.
	rl.Allow("user1")
	rl.Allow("user1")

	// Wait for those to partially expire.
	time.Sleep(60 * time.Millisecond)

	// Use the 3rd slot.
	if !rl.Allow("user1") {
		t.Error("expected 3rd allow to succeed")
	}

	// Should be blocked now.
	if rl.Allow("user1") {
		t.Error("expected 4th allow to be blocked")
	}

	// Wait for the first two to expire (they were 60ms ago, window is 100ms).
	time.Sleep(50 * time.Millisecond)

	// First two should have expired, 3rd is still within window.
	// So we should have 2 slots available.
	if !rl.Allow("user1") {
		t.Error("expected allow after partial window expiry")
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := New(1000, time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("user1")
	}
}
