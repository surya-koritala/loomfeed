package loom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/models"
)

// cacheVersion is bumped whenever the cached payload shape changes
// (e.g. we start storing token counts alongside the text) or the
// underlying prompt changes meaningfully. All cache reads with a
// different version miss, which forces a fresh LLM call. Cheaper
// than a SCAN+UNLINK over the namespace.
//
// v2 (2026-05-13): tightened summarize prompt — dropped inline
// disclaimer (UI footer renders one), capped at 80 words. Old v1
// entries were verbose + duplicated the disclaimer, so they should
// not be replayed.
const cacheVersion = "v2"

// defaultCacheTTL keeps a summarize for an hour. Post bodies rarely
// change mid-thread; future intents like fact_check may want a longer
// TTL keyed on the specific claim being checked.
const defaultCacheTTL = time.Hour

// CachedResponse is what we serialize into Redis. The model used and
// token counts are kept alongside the text so a cache hit can still
// log a faithful cost row (cost_usd = 0 but model + tokens preserved).
type CachedResponse struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// cacheKey computes the cache slot for an (intent, scope, content)
// triple. scope is the surface the prompt is bound to — typically the
// post ID — so two posts with identical bodies don't share a slot.
// The content hash captures what the LLM was actually asked to
// process; same content + same scope + same intent = cache hit.
func cacheKey(intent models.LoomIntent, scope, content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return fmt.Sprintf("loom:cache:%s:%s:%s:%s", cacheVersion, intent, scope, hex.EncodeToString(h.Sum(nil)))
}

// GetCached reads from Redis. Miss + transport error both return
// (nil, nil) — degraded mode is "always miss, never cache," which
// stays correct, just slightly costlier.
func GetCached(ctx context.Context, c *cache.RedisCache, intent models.LoomIntent, scope, content string) (*CachedResponse, error) {
	if c == nil {
		return nil, nil
	}
	raw, err := c.Get(ctx, cacheKey(intent, scope, content))
	if err != nil || raw == nil {
		return nil, nil
	}
	var cr CachedResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, nil
	}
	return &cr, nil
}

// SetCached writes the response with defaultCacheTTL. Fire-and-forget
// — the underlying RedisCache.Set already short-circuits on a nil
// receiver and treats slow Redis as a miss.
func SetCached(ctx context.Context, c *cache.RedisCache, intent models.LoomIntent, scope, content string, resp CachedResponse) {
	if c == nil {
		return
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = c.Set(ctx, cacheKey(intent, scope, content), raw, defaultCacheTTL)
}
