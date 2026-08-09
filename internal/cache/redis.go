package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache wraps a Redis client with simple Get/Set/Delete operations
// for caching serialized response data.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new RedisCache. The client may be nil, in which
// case all operations are safe no-ops (graceful degradation).
func NewRedisCache(client *redis.Client) *RedisCache {
	if client == nil {
		return nil
	}
	return &RedisCache{client: client}
}

// Get retrieves cached data by key. Returns nil, nil on cache miss,
// timeout, or when called on a nil receiver (degraded-mode safe).
// Uses a 500ms timeout to avoid blocking requests when Redis is slow.
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	data, err := c.client.Get(rctx, key).Bytes()
	if err == redis.Nil || err != nil {
		return nil, nil // treat all errors as cache miss
	}
	return data, nil
}

// Set stores data with a TTL. Fire-and-forget with 500ms timeout.
// No-op when called on a nil receiver (degraded-mode safe).
func (c *RedisCache) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	if c == nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return c.client.Set(rctx, key, data, ttl).Err()
}

// Delete removes one or more keys. No-op when called on a nil
// receiver (degraded-mode safe).
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if c == nil || len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Versioned cache keys — atomic O(1) invalidation that doesn't have
// to SCAN+UNLINK the entire namespace.
//
// Each namespace ("feed", "activity", "community") has an integer
// counter stored at `cache:ver:<namespace>`. Real cache keys embed
// the current counter value: `<namespace>:<version>:<subkey>`.
//
// To invalidate the namespace, BumpVersion increments the counter.
// New reads + writes naturally use the new value; old keys are
// unreachable and age out via their existing TTL. No SCAN, no
// DEL/UNLINK fan-out — one atomic INCR.
//
// The version is fetched server-side via Lua so reads + writes stay
// at one client→server round trip (the version GET and the data
// GET/SET happen in a single command).

const versionKeyPrefix = "cache:ver:"

// getVersionedScript reads the namespace version and then the
// versioned data key. Returns nil on data miss.
var getVersionedScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then v = '0' end
return redis.call('GET', KEYS[2] .. ':' .. v .. ':' .. ARGV[1])
`)

// setVersionedScript reads the namespace version, then SETs the
// versioned key with TTL.
var setVersionedScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then v = '0' end
return redis.call('SET', KEYS[2] .. ':' .. v .. ':' .. ARGV[1], ARGV[2], 'EX', tonumber(ARGV[3]))
`)

// GetVersioned reads <namespace>:<version>:<subkey>, where the
// version is fetched atomically alongside the data lookup. Returns
// nil, nil on miss or any transport error (degraded-mode safe).
//
// Nil-receiver safe.
func (c *RedisCache) GetVersioned(ctx context.Context, namespace, subkey string) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	v, err := getVersionedScript.Run(rctx, c.client,
		[]string{versionKeyPrefix + namespace, namespace}, subkey).Result()
	if err != nil {
		// Treat all errors (miss, timeout, transport) as a cache miss.
		return nil, nil
	}
	// Lua scripts that return a Redis bulk string come back as Go
	// `string` from go-redis. A nil reply (key missing) surfaces as
	// redis.Nil err — already handled above.
	if s, ok := v.(string); ok {
		return []byte(s), nil
	}
	return nil, nil
}

// SetVersioned writes <namespace>:<version>:<subkey> with the given
// TTL. If invalidation happens concurrently (BumpVersion called in
// flight), the write may land in a soon-to-be-orphaned bucket — but
// that just means the data ages out a bit faster. No correctness
// issue. Nil-receiver safe.
func (c *RedisCache) SetVersioned(ctx context.Context, namespace, subkey string, data []byte, ttl time.Duration) error {
	if c == nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return setVersionedScript.Run(rctx, c.client,
		[]string{versionKeyPrefix + namespace, namespace},
		subkey, data, int(ttl.Seconds())).Err()
}

// BumpVersion increments the namespace's version counter, effectively
// invalidating every key under that namespace in O(1). Old keys are
// not deleted; they age out via TTL. The counter has no TTL itself
// (we want it to live across restarts so cache state stays
// consistent). Nil-receiver safe.
func (c *RedisCache) BumpVersion(ctx context.Context, namespace string) error {
	if c == nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return c.client.Incr(rctx, versionKeyPrefix+namespace).Err()
}

// incrWindowScript atomically increments a fixed-window counter and,
// only on the first increment of the window, sets its expiry. Returning
// the post-increment count lets the caller decide allow/deny without a
// second round trip. EXPIRE-on-first means the window slides forward
// from the first request in each window, not from every request.
var incrWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
	redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// IncrWindow increments the fixed-window counter at key, setting a
// `window` TTL on the first increment, and returns the new count.
//
// Used for cross-replica rate limiting: every replica INCRs the same
// Redis key, so the limit is enforced cluster-wide rather than
// per-process. Errors (including a nil receiver / Redis down) are
// surfaced to the caller, which is expected to FAIL OPEN — availability
// beats strict enforcement for a rate limiter. 500ms timeout so a slow
// Redis can't stall the request path.
func (c *RedisCache) IncrWindow(ctx context.Context, key string, window time.Duration) (int64, error) {
	if c == nil {
		return 0, redis.ErrClosed
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return incrWindowScript.Run(rctx, c.client, []string{key}, int(window.Seconds())).Int64()
}

// PeekCount returns the current value of a counter key without
// modifying it (0 on miss). Used to compute X-RateLimit-Remaining.
// Nil-receiver / error safe: returns (0, err) and the caller treats
// any error as "full budget remaining".
func (c *RedisCache) PeekCount(ctx context.Context, key string) (int64, error) {
	if c == nil {
		return 0, redis.ErrClosed
	}
	rctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	n, err := c.client.Get(rctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

// DeletePattern removes all keys matching a glob pattern (e.g. "feed:*").
// Uses SCAN + batched UNLINK so we don't block Redis with a KEYS sweep
// and don't pay one round-trip per key the way the old DEL-per-iter
// path did.
//
// Three things matter for the hot path (every vote/post/comment fires
// "feed:*"):
//
//  1. SCAN cursor batch size is 500 (was 100) → ~5x fewer SCAN
//     round-trips on a 10k-key namespace.
//  2. UNLINK instead of DEL → the server reclaims memory in a
//     background thread, the foreground reply returns instantly.
//  3. Keys are batched into chunks of 500 and sent in one UNLINK call
//     → one client-server round-trip per 500 keys instead of one per
//     key (was the dominant cost under vote bursts).
//  4. A 5s overall timeout — under a Redis brownout we prefer to
//     return early and let surviving keys TTL-expire than hold up
//     the request thread.
//
// Nil-receiver safe (degraded mode).
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	if c == nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const batchSize = 500
	iter := c.client.Scan(rctx, 0, pattern, batchSize).Iterator()

	batch := make([]string, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := c.client.Unlink(rctx, batch...).Err()
		batch = batch[:0]
		return err
	}

	for iter.Next(rctx) {
		batch = append(batch, iter.Val())
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return iter.Err()
}
