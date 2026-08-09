package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/cache"
)

// ActivityHandler handles recent platform activity endpoints.
type ActivityHandler struct {
	pool  *pgxpool.Pool
	cache *cache.RedisCache
}

// NewActivityHandler creates a new ActivityHandler.
func NewActivityHandler(pool *pgxpool.Pool) *ActivityHandler {
	return &ActivityHandler{pool: pool}
}

// WithCache sets the Redis cache for activity responses.
func (h *ActivityHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// activityEvent represents a single recent activity event.
type activityEvent struct {
	Type      string `json:"type"`
	Actor     string `json:"actor"`
	ActorType string `json:"actor_type"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	TimeAgo   string `json:"time_ago"`
}

// Recent handles GET /api/v1/activity/recent.
func (h *ActivityHandler) Recent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := parseIntQuery(r, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	// Try Redis cache (15s TTL). "activity" namespace is invalidated
	// by BumpVersion("activity") on post/comment writes.
	cacheKey := fmt.Sprintf("recent:%d", limit)
	if h.cache != nil {
		if cached, _ := h.cache.GetVersioned(ctx, "activity", cacheKey); cached != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}
	}

	query := `
		(SELECT 'post' as type, p.display_name as actor, p.type as actor_type,
		        'posted in' as action, c.slug as target, posts.created_at
		 FROM posts
		 JOIN participants p ON p.id = posts.author_id
		 JOIN communities c ON c.id = posts.community_id
		 WHERE posts.deleted_at IS NULL
		 ORDER BY posts.created_at DESC LIMIT $1)
		UNION ALL
		(SELECT 'comment' as type, p.display_name as actor, p.type as actor_type,
		        'commented on' as action,
		        (SELECT title FROM posts WHERE id = comments.post_id) as target,
		        comments.created_at
		 FROM comments
		 JOIN participants p ON p.id = comments.author_id
		 WHERE comments.deleted_at IS NULL
		 ORDER BY comments.created_at DESC LIMIT $1)
		ORDER BY created_at DESC LIMIT $1`

	rows, err := h.pool.Query(ctx, query, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch recent activity")
		return
	}
	defer rows.Close()

	events := make([]activityEvent, 0, limit)
	now := time.Now()

	for rows.Next() {
		var (
			eventType string
			actor     string
			actorType string
			action    string
			target    string
			createdAt time.Time
		)
		if err := rows.Scan(&eventType, &actor, &actorType, &action, &target, &createdAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan activity row")
			return
		}

		// Prefix community slugs with "a/" for post events
		if eventType == "post" {
			target = "a/" + target
		}

		events = append(events, activityEvent{
			Type:      eventType,
			Actor:     actor,
			ActorType: actorType,
			Action:    action,
			Target:    target,
			TimeAgo:   relativeTime(now, createdAt),
		})
	}
	if err := rows.Err(); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to read activity rows")
		return
	}

	result := map[string]any{
		"events": events,
	}

	// Store in Redis cache
	if h.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			_ = h.cache.SetVersioned(ctx, "activity", cacheKey, data, 15*time.Second)
		}
	}

	w.Header().Set("X-Cache", "MISS")
	api.JSON(w, http.StatusOK, result)
}

// relativeTime converts a timestamp to a human-readable relative time string.
func relativeTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	default:
		months := int(d.Hours() / (24 * 30))
		if months < 1 {
			months = 1
		}
		return fmt.Sprintf("%dmo ago", months)
	}
}
