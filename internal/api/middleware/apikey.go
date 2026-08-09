package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// hashAPIKey returns the cache key we use to refer to an API key
// across replicas without exposing the plaintext to Redis. SHA-256
// is fine here — collisions vanish before we care, and we never
// need to invert the hash.
func hashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return "apikey:neg:" + hex.EncodeToString(sum[:])
}

// defaultKeyRateLimit is the per-minute request budget applied to an
// API key whose stored rate_limit is unset (<= 0). Matches the
// api_keys.rate_limit column default (see migration 000085) and sits
// above every per-action limit so it acts as an abuse backstop rather
// than a throttle on normal agent activity.
const defaultKeyRateLimit = 300

// keyQuotaKey is the Redis counter key for an API key's per-minute
// request quota. Keyed by the SHA-256 of the plaintext key (never the
// key itself) so it's precise per-key and available on every code path
// — both the cache-hit fast path and the fresh-resolve path have the
// plaintext in hand.
func keyQuotaKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return "kq:" + hex.EncodeToString(sum[:])
}

// allowKeyQuota enforces the per-key request budget cluster-wide via a
// shared Redis fixed-window counter. It FAILS OPEN: a nil cache (Redis
// not configured) or any Redis error returns true, so the quota never
// takes the API down with Redis. limit <= 0 falls back to the default.
func allowKeyQuota(ctx context.Context, sharedCache *cache.RedisCache, apiKey string, limit int) bool {
	if sharedCache == nil {
		return true
	}
	if limit <= 0 {
		limit = defaultKeyRateLimit
	}
	count, err := sharedCache.IncrWindow(ctx, keyQuotaKey(apiKey), time.Minute)
	if err != nil {
		return true // fail open
	}
	return count <= int64(limit)
}

// allowAgentQuota enforces the per-AGENT request budget
// (agent_identities.max_rpm) cluster-wide. Unlike the key quota it is
// keyed by participant ID, so it spans every key the agent owns.
// maxRPM <= 0 means the agent has no cap (only the key quota applies).
// Same fail-open semantics as allowKeyQuota.
func allowAgentQuota(ctx context.Context, sharedCache *cache.RedisCache, participantID string, maxRPM int) bool {
	if sharedCache == nil || maxRPM <= 0 {
		return true
	}
	count, err := sharedCache.IncrWindow(ctx, "aq:"+participantID, time.Minute)
	if err != nil {
		return true // fail open
	}
	return count <= int64(maxRPM)
}

// processWideAPIKeyCache is a SINGLE in-process positive/negative
// cache shared across every APIKeyAuth instance in this process.
//
// Each `requireAnyAuth(...)` route registration calls APIKeyAuth(),
// which used to create its own *apiKeyCache. The result was
// per-route cache isolation: an agent's first call to /posts warmed
// only that route's cache, so the same agent's first call to /votes
// or /auth/me had to pay the full bcrypt scan again. With ~80 keys
// at ~100ms per bcrypt on a 0.5-CPU container, that's 16s per
// route per agent. End result: agent vote calls were timing out
// inside the MCP→API gateway hop ("context canceled" cascade).
//
// The package-level singleton lets every route share validations
// across the process. Per-replica is still per-replica (Redis
// negative cache covers cluster-wide invalidations), but at least
// each replica only pays the scan once per agent, not once per
// agent per route.
var processWideAPIKeyCache = newAPIKeyCache()

// failedAttempts tracks failed API key attempts per IP for brute-force protection.
// Keys are "apikeyFail:<ip>", values are *failedAttemptEntry.
var failedAttempts sync.Map

type failedAttemptEntry struct {
	count     int64
	resetAt   time.Time
}

const maxFailedAttempts = 10
const failedAttemptWindow = 5 * time.Minute

func checkAndRecordFailedAttempt(r *http.Request) bool {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	key := "apikeyFail:" + ip

	if val, ok := failedAttempts.Load(key); ok {
		entry := val.(*failedAttemptEntry)
		if time.Now().After(entry.resetAt) {
			// Window expired, reset
			failedAttempts.Delete(key)
		} else if entry.count >= int64(maxFailedAttempts) {
			return true // rate limited
		}
	}

	// Increment failure count
	if val, ok := failedAttempts.Load(key); ok {
		entry := val.(*failedAttemptEntry)
		entry.count++
	} else {
		failedAttempts.Store(key, &failedAttemptEntry{
			count:   1,
			resetAt: time.Now().Add(failedAttemptWindow),
		})
	}
	return false
}

// apiKeyCache caches API key validation results — both successful
// (claims, 5 min TTL) and unsuccessful (sentinel, 60 sec TTL).
//
// Negative caching matters: without it, every invalid-key request
// pays the full prefix-miss + legacy bcrypt scan cost. With ~80
// active keys at ~100ms per bcrypt, that's 8s per attempt — long
// enough to look like a hang and exceed Cloudflare's stream window.
// 60 sec is short enough that genuine key rotations (admin
// reactivates a key) propagate quickly while still amortizing the
// cost over thousands of repeat-attempt requests.
type apiKeyCache struct {
	mu      sync.RWMutex
	entries map[string]apiKeyCacheEntry
}

type apiKeyCacheEntry struct {
	claims    *auth.Claims // nil = negative cache (key is invalid)
	expiresAt time.Time
}

func newAPIKeyCache() *apiKeyCache {
	c := &apiKeyCache{entries: make(map[string]apiKeyCacheEntry)}
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			c.mu.Lock()
			now := time.Now()
			for k, e := range c.entries {
				if now.After(e.expiresAt) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		}
	}()
	return c
}

// cacheStatus is what cache.get returns. ok=true means we have a
// cached answer; claims is non-nil for valid keys, nil for known-bad.
type cacheStatus struct {
	ok     bool
	claims *auth.Claims
}

func (c *apiKeyCache) get(key string) cacheStatus {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return cacheStatus{ok: false}
	}
	return cacheStatus{ok: true, claims: e.claims}
}

func (c *apiKeyCache) set(key string, claims *auth.Claims) {
	c.mu.Lock()
	c.entries[key] = apiKeyCacheEntry{claims: claims, expiresAt: time.Now().Add(5 * time.Minute)}
	c.mu.Unlock()
}

func (c *apiKeyCache) setInvalid(key string) {
	c.mu.Lock()
	c.entries[key] = apiKeyCacheEntry{claims: nil, expiresAt: time.Now().Add(60 * time.Second)}
	c.mu.Unlock()
}

// APIKeyAuth validates API keys from the X-API-Key header OR Authorization: Bearer ak_... header.
// Caches successful validations for 5 minutes to avoid repeated DB + bcrypt overhead.
func APIKeyAuth(apikeys *repository.APIKeyRepo, sharedCache *cache.RedisCache) func(http.Handler) http.Handler {
	cache := processWideAPIKeyCache

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")

			// Also check Authorization: Bearer ak_... (many agents use this format)
			if apiKey == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ak_") {
					apiKey = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check in-process positive cache first — fastest path,
			// successful auths cached for 5 min per replica. The cached
			// claims carry the key's rate_limit, so we still enforce the
			// per-key quota here without re-resolving the key.
			if hit := cache.get(apiKey); hit.ok && hit.claims != nil {
				if !allowKeyQuota(r.Context(), sharedCache, apiKey, hit.claims.RateLimit) {
					w.Header().Set("Retry-After", "60")
					http.Error(w, `{"error":"API key rate limit exceeded"}`, http.StatusTooManyRequests)
					return
				}
				if !allowAgentQuota(r.Context(), sharedCache, hit.claims.ParticipantID, hit.claims.MaxRPM) {
					w.Header().Set("Retry-After", "60")
					http.Error(w, `{"error":"agent rate limit exceeded (max_rpm)"}`, http.StatusTooManyRequests)
					return
				}
				ctx := context.WithValue(r.Context(), ClaimsKey, hit.claims)
				ctx = database.WithUserID(ctx, hit.claims.ParticipantID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Shared (Redis) negative cache — survives replica
			// boundaries and process restarts. After ANY replica
			// determines a key is invalid, every replica sees the
			// rejection in ms instead of paying the bcrypt scan.
			// 0.5-CPU containers make this load-bearing: a sequential
			// 80-key bcrypt sweep wallclocks at ~16s per replica
			// without it.
			negCacheKey := hashAPIKey(apiKey)
			if sharedCache != nil {
				if data, _ := sharedCache.Get(r.Context(), negCacheKey); len(data) > 0 {
					if checkAndRecordFailedAttempt(r) {
						http.Error(w, `{"error":"too many failed attempts, try again later"}`, http.StatusTooManyRequests)
						return
					}
					http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
					return
				}
			}

			// Per-replica negative cache (for when Redis is down or
			// not configured). Same idea, smaller blast radius.
			if hit := cache.get(apiKey); hit.ok && hit.claims == nil {
				if checkAndRecordFailedAttempt(r) {
					http.Error(w, `{"error":"too many failed attempts, try again later"}`, http.StatusTooManyRequests)
					return
				}
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			// Cache miss — try fast prefix lookup first (O(1)), fallback to full scan
			var matchedAgentID string
			var matchedScopes []string
			var matchedRateLimit int
			var matchedMaxRPM int

			// Extract prefix: "ak_" + first 16 hex chars
			if len(apiKey) >= 19 { // "ak_" + at least 16 chars
				prefix := apiKey[3:19]
				key, err := apikeys.GetByPrefix(r.Context(), prefix)
				if err == nil && key != nil {
					if bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(apiKey)) == nil {
						matchedAgentID = key.AgentID
						matchedScopes = key.Scopes
						matchedRateLimit = key.RateLimit
						matchedMaxRPM = key.AgentMaxRPM
					}
				}
			}

			// Fallback: full scan of all active keys via bcrypt. We
			// can't restrict this to "legacy keys without a prefix"
			// — that breaks any key whose stored key_prefix doesn't
			// match the prefix we'd extract today (older derivation
			// logic, manual DB edits, partially-completed migrations).
			//
			// Performance: parallelize the bcrypt comparisons so
			// wallclock latency is bounded by max(bcrypt_one_key)
			// ≈ 100ms regardless of how many keys we have. Sequential
			// scan was 80 keys × 100ms = 8s, blew Cloudflare's stream
			// window, and looked like a hang.
			if matchedAgentID == "" {
				keys, err := apikeys.GetAllActive(r.Context())
				if err != nil {
					http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
					return
				}
				type matchResult struct {
					id        string
					scopes    []string
					keyID     string
					rateLimit int
					maxRPM    int
				}
				resCh := make(chan matchResult, 1)
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()

				var wg sync.WaitGroup
				for _, k := range keys {
					wg.Add(1)
					go func(k models.APIKey) {
						defer wg.Done()
						// Cheap context check up front — if the
						// match has already arrived on another
						// goroutine, don't bother spending bcrypt
						// CPU on this one.
						if ctx.Err() != nil {
							return
						}
						if bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(apiKey)) == nil {
							select {
							case resCh <- matchResult{id: k.AgentID, scopes: k.Scopes, keyID: k.ID, rateLimit: k.RateLimit, maxRPM: k.AgentMaxRPM}:
							default:
								// Another goroutine already won.
							}
						}
					}(k)
				}

				go func() {
					wg.Wait()
					close(resCh)
				}()

				if r, ok := <-resCh; ok {
					matchedAgentID = r.id
					matchedScopes = r.scopes
					matchedRateLimit = r.rateLimit
					matchedMaxRPM = r.maxRPM
					cancel()
					// Backfill / repair prefix for future fast lookups
					if len(apiKey) >= 19 {
						_ = apikeys.SetPrefix(context.Background(), r.keyID, apiKey[3:19])
					}
				}
			}

			// Successful match — purge any negative cache entry that
			// might be lingering for this key (e.g. from a previous
			// canceled request). Without this, a key that just got
			// rescued from misclassification would still be locked
			// out of OTHER replicas until their negative-cache TTL
			// expires.
			if matchedAgentID != "" && sharedCache != nil {
				_ = sharedCache.Delete(context.Background(), negCacheKey)
			}

			if matchedAgentID == "" {
				// CRITICAL: only cache "key is invalid" if we actually
				// finished checking. If the request context was
				// canceled (client disconnect, gateway timeout, scan
				// aborted) the parallel scan goroutines bail out
				// early — no match arrives on the channel, but the
				// key is *not* known to be invalid. Caching it as
				// invalid here would lock a perfectly valid key out
				// of the cluster for the negative-cache TTL.
				//
				// We saw this in production: agent's real key worked
				// for create_post, then failed cluster-wide for
				// whoami/heartbeat/etc with "invalid API key" because
				// an earlier canceled request poisoned the Redis
				// negative cache.
				if r.Context().Err() == nil {
					cache.setInvalid(apiKey)
					if sharedCache != nil {
						// 15s TTL (was 60s) — short enough that any
						// future correctness slip heals quickly,
						// long enough to absorb a brute-force burst
						// against a fixed bad key.
						_ = sharedCache.Set(context.Background(), negCacheKey, []byte("1"), 15*time.Second)
					}
				}
				if checkAndRecordFailedAttempt(r) {
					http.Error(w, `{"error":"too many failed attempts, try again later"}`, http.StatusTooManyRequests)
					return
				}
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			claims := &auth.Claims{
				ParticipantID:   matchedAgentID,
				ParticipantType: "agent",
				Scopes:          matchedScopes,
				RateLimit:       matchedRateLimit,
				MaxRPM:          matchedMaxRPM,
			}

			// Cache the result
			cache.set(apiKey, claims)

			// Track last_used_at asynchronously (once per cache cycle, not every request)
			prefix := ""
			if len(apiKey) >= 19 {
				prefix = apiKey[3:19]
			}
			if prefix != "" {
				go func() {
					_ = apikeys.UpdateLastUsed(context.Background(), prefix)
				}()
			}

			// Per-key request quota (api_keys.rate_limit). Enforced after
			// caching so a valid key stays warm even when this request is
			// the one that trips the limit.
			if !allowKeyQuota(r.Context(), sharedCache, apiKey, claims.RateLimit) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"API key rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			// Per-agent request quota (agent_identities.max_rpm) — spans
			// all of the agent's keys.
			if !allowAgentQuota(r.Context(), sharedCache, claims.ParticipantID, claims.MaxRPM) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"agent rate limit exceeded (max_rpm)"}`, http.StatusTooManyRequests)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = database.WithUserID(ctx, claims.ParticipantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CombinedAuth chains APIKeyAuth and JWT Auth so that either credential type is accepted.
// API key auth is checked first; if no X-API-Key header is present, JWT auth takes over.
// sharedCache is optional — pass nil to fall back to per-replica negative caching.
func CombinedAuth(apikeys *repository.APIKeyRepo, jwtSecret string, sharedCache *cache.RedisCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return APIKeyAuth(apikeys, sharedCache)(Auth(jwtSecret)(next))
	}
}

// RequireScope returns middleware that checks if the authenticated participant has the required scope.
// JWT-authenticated users (no scopes set) are always allowed through.
// API key-authenticated agents must have the required scope in their key's scopes list.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"missing auth claims"}`, http.StatusUnauthorized)
				return
			}

			// If scopes are set (API key auth), check for the required scope
			if len(claims.Scopes) > 0 && !containsScope(claims.Scopes, scope) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func containsScope(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}
