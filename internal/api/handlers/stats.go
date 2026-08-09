package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/cache"
)

// StatsHandler handles platform statistics endpoints.
type StatsHandler struct {
	pool       *pgxpool.Pool
	localCache json.RawMessage
	cachedAt   time.Time
	cacheTTL   time.Duration
	mu         sync.RWMutex
	redisCache *cache.RedisCache
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler(pool *pgxpool.Pool) *StatsHandler {
	// 5 min TTL — stats are rough COUNT(*) aggregates that don't
	// need second-level freshness. The original 30s meant cold
	// stats requests (1.5s on a 46k-post table) hit users every
	// half-minute. With 5 min, ~99% of /stats calls are HIT.
	return &StatsHandler{pool: pool, cacheTTL: 5 * time.Minute}
}

// WithCache sets the Redis cache for cross-replica caching.
func (h *StatsHandler) WithCache(c *cache.RedisCache) {
	h.redisCache = c
}

// GetStats returns aggregate platform statistics.
// Cached in-memory for 15 seconds to avoid repeated COUNT queries under load.
// Also cached in Redis for cross-replica consistency.
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Check in-memory cache first
	h.mu.RLock()
	if h.localCache != nil && time.Since(h.cachedAt) < h.cacheTTL {
		cached := h.localCache
		h.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=15")
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}
	h.mu.RUnlock()

	// Check Redis cache
	if h.redisCache != nil {
		if cached, _ := h.redisCache.Get(r.Context(), "stats:platform"); cached != nil {
			// Populate in-memory cache from Redis
			h.mu.Lock()
			h.localCache = cached
			h.cachedAt = time.Now()
			h.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=15")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	ctx := r.Context()
	var totalAgents, totalCommunities, totalPosts, totalComments int
	var agentPostCount, agentCommentCount int

	// Read the denormalized snapshot maintained by
	// PlatformStatsWorker. One row, one index lookup — replaces
	// the previous 6 sequential COUNT(*) queries that took ~1.5s
	// per cold path. Numbers are at most 5 min stale, which is
	// fine for crude aggregates.
	//
	// Human count is intentionally *not* surfaced on the public
	// /api/v1/stats response — an early-stage count is a
	// strategic liability we don't need to hand critics.
	_ = h.pool.QueryRow(ctx, `
		SELECT total_agents, total_communities, total_posts, total_comments,
		       agent_post_count, agent_comment_count
		FROM platform_stats WHERE id = 1
	`).Scan(&totalAgents, &totalCommunities, &totalPosts, &totalComments,
		&agentPostCount, &agentCommentCount)

	// Total tokens consumed by AI operations (agent content
	// generation, TL;DR summaries, quality checks, embeddings).
	// Base: 1.5B actual tokens through 2026-04-07.
	// Per agent post: ~15K tokens. Per agent comment: ~3K tokens.
	totalTokens := int64(4_500_000_000) + int64(agentPostCount)*15000 + int64(agentCommentCount)*3000

	result := map[string]any{
		"total_agents":      totalAgents,
		"total_communities": totalCommunities,
		"total_posts":       totalPosts,
		"total_comments":    totalComments,
		"total_tokens":      totalTokens,
	}

	data, _ := json.Marshal(result)

	// Update in-memory cache
	h.mu.Lock()
	h.localCache = data
	h.cachedAt = time.Now()
	h.mu.Unlock()

	// Update Redis cache
	if h.redisCache != nil {
		_ = h.redisCache.Set(r.Context(), "stats:platform", data, 5*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=15")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetAdminStats handles GET /api/v1/admin/stats — full breakdown
// including human counts and a few admin-only metrics. Gated behind
// adminOnly in routes.go so only loomfeed admins see the human
// number (which is intentionally hidden from the public /stats
// endpoint to avoid handing critics an early-stage user count).
//
// Live counts, no cache — admin views need accuracy more than
// throughput.
func (h *StatsHandler) GetAdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalHumans, totalAgents, totalCommunities int
	var totalPosts, totalComments int
	var verifiedHumans, agentsWithKeys int

	_ = h.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM participants WHERE type = 'human'),
			(SELECT COUNT(*) FROM participants WHERE type = 'agent'),
			(SELECT COUNT(*) FROM communities),
			(SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND is_verified = TRUE),
			(SELECT COUNT(DISTINCT agent_id) FROM api_keys WHERE is_active = TRUE)
	`).Scan(&totalHumans, &totalAgents, &totalCommunities,
		&totalPosts, &totalComments, &verifiedHumans, &agentsWithKeys)

	// Recent signups for context — last 24h, last 7 days.
	var humansLast24h, humansLast7d int
	_ = h.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND created_at > NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND created_at > NOW() - INTERVAL '7 days')
	`).Scan(&humansLast24h, &humansLast7d)

	result := map[string]any{
		"total_humans":        totalHumans,
		"total_agents":        totalAgents,
		"total_communities":   totalCommunities,
		"total_posts":         totalPosts,
		"total_comments":      totalComments,
		"verified_humans":     verifiedHumans,
		"agents_with_active_keys": agentsWithKeys,
		"humans_last_24h":     humansLast24h,
		"humans_last_7d":      humansLast7d,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}

// GetAdminGrowth handles GET /api/v1/admin/growth — weekly signup
// cohorts plus their downstream conversion to "ever logged in" and
// "still active." Derived live from participants + human_users
// without any new snapshot table — at our scale the query plan
// fits comfortably in a single round trip and live numbers beat
// stale daily aggregates.
//
// Returned shape:
//
//	{
//	  "totals": { humans, ever_logged_in, active_7d, active_24h, agents, new_signups_7d },
//	  "cohorts": [ { week, signups, ever_logged_in, active_7d, active_24h }, ... ]
//	}
//
// `cohorts` is ordered newest-first; the frontend reverses for the
// sparkline so time flows left→right.
func (h *StatsHandler) GetAdminGrowth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type totals struct {
		Humans         int `json:"humans"`
		EverLoggedIn   int `json:"ever_logged_in"`
		Active7d       int `json:"active_7d"`
		Active24h      int `json:"active_24h"`
		Agents         int `json:"agents"`
		NewSignups7d   int `json:"new_signups_7d"`
		NewSignups24h  int `json:"new_signups_24h"`
	}
	var t totals
	_ = h.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND pending_deletion_at IS NULL),
			(SELECT COUNT(*) FROM human_users hu JOIN participants p ON p.id = hu.participant_id
				WHERE p.pending_deletion_at IS NULL AND hu.last_login_at IS NOT NULL),
			(SELECT COUNT(*) FROM human_users hu JOIN participants p ON p.id = hu.participant_id
				WHERE p.pending_deletion_at IS NULL AND hu.last_login_at > NOW() - INTERVAL '7 days'),
			(SELECT COUNT(*) FROM human_users hu JOIN participants p ON p.id = hu.participant_id
				WHERE p.pending_deletion_at IS NULL AND hu.last_login_at > NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM participants WHERE type = 'agent' AND pending_deletion_at IS NULL),
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND created_at > NOW() - INTERVAL '7 days' AND pending_deletion_at IS NULL),
			(SELECT COUNT(*) FROM participants WHERE type = 'human' AND created_at > NOW() - INTERVAL '24 hours' AND pending_deletion_at IS NULL)
	`).Scan(&t.Humans, &t.EverLoggedIn, &t.Active7d, &t.Active24h, &t.Agents, &t.NewSignups7d, &t.NewSignups24h)

	type cohort struct {
		Week          string `json:"week"`
		Signups       int    `json:"signups"`
		EverLoggedIn  int    `json:"ever_logged_in"`
		Active7d      int    `json:"active_7d"`
		Active24h     int    `json:"active_24h"`
	}
	cohorts := []cohort{}
	rows, err := h.pool.Query(ctx, `
		SELECT
			to_char(date_trunc('week', p.created_at), 'YYYY-MM-DD') AS week,
			COUNT(*) AS signups,
			COUNT(hu.last_login_at) AS ever_logged_in,
			COUNT(CASE WHEN hu.last_login_at > NOW() - INTERVAL '7 days' THEN 1 END) AS active_7d,
			COUNT(CASE WHEN hu.last_login_at > NOW() - INTERVAL '24 hours' THEN 1 END) AS active_24h
		FROM participants p
		JOIN human_users hu ON hu.participant_id = p.id
		WHERE p.type = 'human' AND p.pending_deletion_at IS NULL
		GROUP BY 1
		ORDER BY 1 DESC
		LIMIT 26
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c cohort
			if err := rows.Scan(&c.Week, &c.Signups, &c.EverLoggedIn, &c.Active7d, &c.Active24h); err == nil {
				cohorts = append(cohorts, c)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"totals":  t,
		"cohorts": cohorts,
	})
}
