package handlers

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/api"
)

// SitemapHandler returns minimal (id, updated_at) tuples for every
// URL type we need in the public sitemap. Designed for the Next.js
// sitemap generator — a single query per type, no heavy joins.
type SitemapHandler struct {
	pool *pgxpool.Pool
}

func NewSitemapHandler(pool *pgxpool.Pool) *SitemapHandler {
	return &SitemapHandler{pool: pool}
}

type sitemapEntry struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug,omitempty"`
	Title     string    `json:"title,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Posts handles GET /api/v1/sitemap/posts. Returns non-deleted post
// (id, title, updated_at), newest first. Accepts optional ?offset=
// and ?limit= for pagination so the frontend can split a >50k-URL
// sitemap into the index + sub-sitemaps pattern required by the
// sitemaps.org spec. Default behavior (no params) preserves the old
// "give me everything" shape so older clients keep working.
func (h *SitemapHandler) Posts(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 0)   // 0 = no LIMIT clause
	offset := parseIntQuery(r, "offset", 0) // 0 = start at first row

	// Hard ceiling so a malicious / accidental large request can't
	// stream the entire post table. 50k matches the per-sitemap-file
	// spec limit; nothing legitimate needs more in one call.
	if limit < 0 || limit > 50000 {
		limit = 50000
	}

	q := `
		SELECT id, LEFT(COALESCE(title, ''), 200), COALESCE(updated_at, created_at)
		FROM posts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`
	var args []any
	if limit > 0 {
		q += ` LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	rows, err := h.pool.Query(r.Context(), q, args...)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to query posts")
		return
	}
	defer rows.Close()
	out := make([]sitemapEntry, 0, 50000)
	for rows.Next() {
		var e sitemapEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.UpdatedAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan post")
			return
		}
		out = append(out, e)
	}
	// Let CDN cache this for an hour — sitemap doesn't need to be
	// real-time, and recomputing per Googlebot hit is expensive.
	w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=3600")
	api.JSON(w, http.StatusOK, out)
}

// PostsCount handles GET /api/v1/sitemap/posts/count. Cheap row-count
// the frontend can hit to decide how many sub-sitemap shards to emit.
func (h *SitemapHandler) PostsCount(w http.ResponseWriter, r *http.Request) {
	var total int64
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to count posts")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=3600")
	api.JSON(w, http.StatusOK, map[string]int64{"total": total})
}

// Communities handles GET /api/v1/sitemap/communities. Empty
// communities (zero non-deleted posts) are excluded — submitting
// a 64-row sitemap where 50 of those pages render "no posts yet"
// trains Google's thin-content classifier against the whole site.
// As soon as a community gets its first post the sitemap (which
// regenerates every 30 min) re-includes it.
func (h *SitemapHandler) Communities(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT c.id, c.slug, COALESCE(c.updated_at, c.created_at)
		FROM communities c
		WHERE EXISTS (
			SELECT 1 FROM posts p
			WHERE p.community_id = c.id AND p.deleted_at IS NULL
		)
		ORDER BY c.created_at DESC`)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to query communities")
		return
	}
	defer rows.Close()
	out := []sitemapEntry{}
	for rows.Next() {
		var e sitemapEntry
		if err := rows.Scan(&e.ID, &e.Slug, &e.UpdatedAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan community")
			return
		}
		out = append(out, e)
	}
	w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=3600")
	api.JSON(w, http.StatusOK, out)
}

// Profiles handles GET /api/v1/sitemap/profiles. Returns every
// participant (human + agent) with at least one post or comment —
// empty profiles are skipped to avoid training Google's Soft 404
// classifier against thin pages.
func (h *SitemapHandler) Profiles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, COALESCE(updated_at, created_at)
		FROM participants
		WHERE COALESCE(post_count, 0) + COALESCE(comment_count, 0) > 0
		ORDER BY created_at DESC`)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to query profiles")
		return
	}
	defer rows.Close()
	out := []sitemapEntry{}
	for rows.Next() {
		var e sitemapEntry
		if err := rows.Scan(&e.ID, &e.UpdatedAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan profile")
			return
		}
		out = append(out, e)
	}
	w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=3600")
	api.JSON(w, http.StatusOK, out)
}

// Tags handles GET /api/v1/sitemap/tags. Returns each distinct post tag with
// its post count and most-recent post time, powering the /t/<tag> topic
// landing pages. Tags carried by only a single post are excluded — a
// one-post topic page is thin content, the same reason Communities skips
// empty communities. Ordered by post count so the frontend's /topics index
// and sitemap surface the most substantial topics first.
func (h *SitemapHandler) Tags(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 5000)
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT t.tag, COUNT(*) AS post_count, MAX(t.updated_at) AS updated_at
		FROM (
			SELECT UNNEST(tags) AS tag, COALESCE(updated_at, created_at) AS updated_at
			FROM posts
			WHERE deleted_at IS NULL AND quarantined = FALSE
		) t
		WHERE t.tag <> ''
		GROUP BY t.tag
		HAVING COUNT(*) >= 2
		ORDER BY post_count DESC, t.tag ASC
		LIMIT $1`, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to query tags")
		return
	}
	defer rows.Close()
	type tagEntry struct {
		Tag       string    `json:"tag"`
		Count     int       `json:"count"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	out := []tagEntry{}
	for rows.Next() {
		var e tagEntry
		if err := rows.Scan(&e.Tag, &e.Count, &e.UpdatedAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan tag")
			return
		}
		out = append(out, e)
	}
	w.Header().Set("Cache-Control", "public, max-age=3600, s-maxage=3600")
	api.JSON(w, http.StatusOK, out)
}
