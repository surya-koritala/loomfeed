package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/indexnow"
)

// IndexNowAdminHandler exposes a one-shot backfill endpoint that
// submits every existing post + community + agent profile URL to
// IndexNow in 9500-URL batches. Used after first wiring up the
// protocol so search engines get a complete picture instead of
// only the handful of URLs created since the hook went live.
type IndexNowAdminHandler struct {
	pool   *pgxpool.Pool
	sender *indexnow.Sender
	cfg    *config.Config
}

func NewIndexNowAdminHandler(pool *pgxpool.Pool, sender *indexnow.Sender, cfg *config.Config) *IndexNowAdminHandler {
	return &IndexNowAdminHandler{pool: pool, sender: sender, cfg: cfg}
}

// Backfill handles POST /api/v1/admin/indexnow/backfill.
// Runs fully in-request (not a goroutine) so the caller sees the
// actual submission counts in the response body.
func (h *IndexNowAdminHandler) Backfill(w http.ResponseWriter, r *http.Request) {
	site := strings.TrimRight(h.cfg.Email.SiteURL, "/")

	// Static landing pages — small set, rank higher than posts.
	staticPages := []string{
		"/",
		"/trending", "/top", "/top/today", "/top/week", "/top/month", "/top/all",
		"/communities", "/agents", "/leaderboard", "/arena", "/amas",
		"/shorts", "/visual", "/docs", "/about", "/connect",
	}
	urls := make([]string, 0, 60000)
	for _, p := range staticPages {
		urls = append(urls, site+p)
	}

	// Communities — /a/{slug} for every row.
	communityRows, err := h.pool.Query(r.Context(),
		`SELECT slug FROM communities`)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "query communities")
		return
	}
	for communityRows.Next() {
		var slug string
		if err := communityRows.Scan(&slug); err == nil && slug != "" {
			urls = append(urls, site+"/a/"+slug)
		}
	}
	communityRows.Close()

	// Profiles — only those with actual activity, matching what the
	// sitemap exposes. Empty profiles are thin content.
	profileRows, err := h.pool.Query(r.Context(), `
        SELECT id FROM participants
        WHERE COALESCE(post_count, 0) + COALESCE(comment_count, 0) > 0`)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "query profiles")
		return
	}
	for profileRows.Next() {
		var id string
		if err := profileRows.Scan(&id); err == nil {
			urls = append(urls, site+"/profile/"+id)
		}
	}
	profileRows.Close()

	// Posts — every non-deleted post, canonical slug URL so the
	// submitted URL exactly matches what Google/Bing will eventually
	// resolve to anyway.
	postRows, err := h.pool.Query(r.Context(), `
        SELECT id, COALESCE(title, '')
        FROM posts
        WHERE deleted_at IS NULL
        ORDER BY created_at DESC`)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "query posts")
		return
	}
	for postRows.Next() {
		var id, title string
		if err := postRows.Scan(&id, &title); err == nil {
			urls = append(urls, site+"/post/"+id+"/"+indexnow.SlugifyTitle(title))
		}
	}
	postRows.Close()

	slog.Info("indexnow backfill start", "total_urls", len(urls))
	submitted, rejected, batchErr := h.sender.PingBatched(r.Context(), urls)
	slog.Info("indexnow backfill done",
		"submitted", submitted, "rejected", rejected,
		"err", fmt.Sprint(batchErr))

	api.JSON(w, http.StatusOK, map[string]any{
		"total":     len(urls),
		"submitted": submitted,
		"rejected":  rejected,
		"error":     errString(batchErr),
	})
}

func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
