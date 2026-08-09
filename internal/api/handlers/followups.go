package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/quality"
)

// FollowupsHandler generates LLM-backed follow-up questions for a post.
// Results are cached in Redis for 24h so a hot post doesn't burn LLM
// budget on every viewer. Cache miss is best-effort — on any failure
// we return an empty list rather than 500.
type FollowupsHandler struct {
	pool  *pgxpool.Pool
	cache *cache.RedisCache
	cfg   *config.Config
}

func NewFollowupsHandler(pool *pgxpool.Pool, cache *cache.RedisCache, cfg *config.Config) *FollowupsHandler {
	return &FollowupsHandler{pool: pool, cache: cache, cfg: cfg}
}

// Get handles GET /api/v1/posts/{id}/followups.
func (h *FollowupsHandler) Get(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	cacheKey := "followups:" + postID
	if h.cache != nil {
		if cached, _ := h.cache.Get(r.Context(), cacheKey); cached != nil {
			api.JSON(w, http.StatusOK, map[string]any{"questions": splitLines(string(cached)), "cached": true})
			return
		}
	}

	var title, body string
	err := h.pool.QueryRow(r.Context(),
		`SELECT title, body FROM posts WHERE id = $1 AND deleted_at IS NULL`, postID).
		Scan(&title, &body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	llmCfg := &quality.LLMConfig{
		Endpoint:       h.cfg.LLM.Endpoint,
		APIKey:         h.cfg.LLM.APIKey,
		DeploymentName: h.cfg.LLM.DeploymentName,
	}
	questions := quality.GenerateFollowups(ctx, title, body, llmCfg)

	if h.cache != nil && len(questions) > 0 {
		_ = h.cache.Set(r.Context(), cacheKey, []byte(strings.Join(questions, "\n")), 24*time.Hour)
	}
	api.JSON(w, http.StatusOK, map[string]any{"questions": questions, "cached": false})
}

func splitLines(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
