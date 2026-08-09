package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestCache spins up an in-memory miniredis and returns a wired
// RedisCache. Callers should `defer mr.Close()` from the returned
// miniredis instance.
func newTestCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisCache(client), mr
}

func TestRedisCache_NilReceiverIsSafe(t *testing.T) {
	var c *RedisCache
	ctx := context.Background()
	if data, err := c.Get(ctx, "k"); data != nil || err != nil {
		t.Errorf("nil Get should return nil, nil; got %v, %v", data, err)
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Errorf("nil Set should be no-op; got %v", err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Errorf("nil Delete should be no-op; got %v", err)
	}
	if err := c.DeletePattern(ctx, "k*"); err != nil {
		t.Errorf("nil DeletePattern should be no-op; got %v", err)
	}
	if data, err := c.GetVersioned(ctx, "ns", "k"); data != nil || err != nil {
		t.Errorf("nil GetVersioned should return nil, nil; got %v, %v", data, err)
	}
	if err := c.SetVersioned(ctx, "ns", "k", []byte("v"), time.Minute); err != nil {
		t.Errorf("nil SetVersioned should be no-op; got %v", err)
	}
	if err := c.BumpVersion(ctx, "ns"); err != nil {
		t.Errorf("nil BumpVersion should be no-op; got %v", err)
	}
}

func TestRedisCache_Versioned_RoundTrip(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	if err := c.SetVersioned(ctx, "feed", "hot:25:0", []byte("payload"), time.Minute); err != nil {
		t.Fatalf("SetVersioned: %v", err)
	}
	got, err := c.GetVersioned(ctx, "feed", "hot:25:0")
	if err != nil {
		t.Fatalf("GetVersioned: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("GetVersioned: got %q, want %q", got, "payload")
	}
}

func TestRedisCache_Versioned_MissReturnsNil(t *testing.T) {
	c, _ := newTestCache(t)
	got, err := c.GetVersioned(context.Background(), "feed", "absent")
	if err != nil {
		t.Errorf("GetVersioned on miss: err=%v, want nil", err)
	}
	if got != nil {
		t.Errorf("GetVersioned on miss: data=%q, want nil", got)
	}
}

func TestRedisCache_BumpVersion_InvalidatesNamespace(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	// Write a value under "feed" namespace
	if err := c.SetVersioned(ctx, "feed", "hot:25:0", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("SetVersioned: %v", err)
	}
	if got, _ := c.GetVersioned(ctx, "feed", "hot:25:0"); string(got) != "v1" {
		t.Fatalf("pre-bump read: got %q, want v1", got)
	}

	// Bump the version — should invalidate the old key.
	if err := c.BumpVersion(ctx, "feed"); err != nil {
		t.Fatalf("BumpVersion: %v", err)
	}
	if got, _ := c.GetVersioned(ctx, "feed", "hot:25:0"); got != nil {
		t.Errorf("post-bump read should miss, got %q", got)
	}

	// And writing at the new version then reading should work.
	if err := c.SetVersioned(ctx, "feed", "hot:25:0", []byte("v2"), time.Minute); err != nil {
		t.Fatalf("SetVersioned v2: %v", err)
	}
	if got, _ := c.GetVersioned(ctx, "feed", "hot:25:0"); string(got) != "v2" {
		t.Errorf("post-bump write: got %q, want v2", got)
	}
}

func TestRedisCache_BumpVersion_NamespaceIsolated(t *testing.T) {
	// Bumping namespace A must not invalidate namespace B.
	c, _ := newTestCache(t)
	ctx := context.Background()

	_ = c.SetVersioned(ctx, "feed", "x", []byte("feed-data"), time.Minute)
	_ = c.SetVersioned(ctx, "activity", "x", []byte("activity-data"), time.Minute)

	if err := c.BumpVersion(ctx, "feed"); err != nil {
		t.Fatalf("BumpVersion feed: %v", err)
	}

	if got, _ := c.GetVersioned(ctx, "feed", "x"); got != nil {
		t.Errorf("feed should be invalidated, got %q", got)
	}
	if got, _ := c.GetVersioned(ctx, "activity", "x"); string(got) != "activity-data" {
		t.Errorf("activity should be untouched, got %q", got)
	}
}

func TestRedisCache_Versioned_RespectsTTL(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	if err := c.SetVersioned(ctx, "feed", "x", []byte("v"), 2*time.Second); err != nil {
		t.Fatalf("SetVersioned: %v", err)
	}
	if got, _ := c.GetVersioned(ctx, "feed", "x"); string(got) != "v" {
		t.Fatalf("pre-expire: got %q", got)
	}
	mr.FastForward(3 * time.Second)
	if got, _ := c.GetVersioned(ctx, "feed", "x"); got != nil {
		t.Errorf("post-expire should miss, got %q", got)
	}
}

func TestRedisCache_GetSetRoundTrip(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	if err := c.Set(ctx, "foo", []byte("bar"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "bar" {
		t.Errorf("Get foo: got %q, want %q", got, "bar")
	}
}

func TestRedisCache_GetMissReturnsNil(t *testing.T) {
	c, _ := newTestCache(t)
	got, err := c.Get(context.Background(), "absent")
	if err != nil {
		t.Errorf("Get on miss: err=%v, want nil", err)
	}
	if got != nil {
		t.Errorf("Get on miss: data=%q, want nil", got)
	}
}

func TestRedisCache_DeletePattern_RemovesAllMatching(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	// Seed enough keys to exercise the multi-batch SCAN path
	// (batchSize is 500, so 1500 forces 3 SCAN-flush iterations).
	const n = 1500
	for i := 0; i < n; i++ {
		if err := c.Set(ctx, fmt.Sprintf("feed:hot:user-%d", i), []byte("x"), time.Minute); err != nil {
			t.Fatalf("seed Set %d: %v", i, err)
		}
	}
	// Plus a few keys that must NOT be deleted.
	for i := 0; i < 5; i++ {
		if err := c.Set(ctx, fmt.Sprintf("activity:user-%d", i), []byte("x"), time.Minute); err != nil {
			t.Fatalf("seed activity: %v", err)
		}
	}

	if err := c.DeletePattern(ctx, "feed:*"); err != nil {
		t.Fatalf("DeletePattern: %v", err)
	}

	// Verify all feed:* are gone.
	feedKeys := mr.Keys()
	for _, k := range feedKeys {
		if len(k) >= 5 && k[:5] == "feed:" {
			t.Errorf("feed key survived DeletePattern: %s", k)
		}
	}

	// Verify activity:* is intact.
	for i := 0; i < 5; i++ {
		got, err := c.Get(ctx, fmt.Sprintf("activity:user-%d", i))
		if err != nil || string(got) != "x" {
			t.Errorf("activity:user-%d should survive DeletePattern, got %q err=%v", i, got, err)
		}
	}
}

func TestRedisCache_DeletePattern_EmptyMatchIsNoOp(t *testing.T) {
	c, _ := newTestCache(t)
	if err := c.DeletePattern(context.Background(), "no-such-pattern:*"); err != nil {
		t.Errorf("DeletePattern with no matches should not error; got %v", err)
	}
}

func TestRedisCache_Delete_MultipleKeys(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	for _, k := range []string{"a", "b", "c"} {
		_ = c.Set(ctx, k, []byte("x"), time.Minute)
	}

	if err := c.Delete(ctx, "a", "b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, k := range []string{"a", "b"} {
		if mr.Exists(k) {
			t.Errorf("%s should be deleted", k)
		}
	}
	if !mr.Exists("c") {
		t.Errorf("c should NOT have been deleted")
	}
}
