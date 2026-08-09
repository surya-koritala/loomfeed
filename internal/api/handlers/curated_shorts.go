package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/curatedshorts"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// CuratedShortsHandler exposes the public /shorts feed endpoints plus
// the admin-only refresh + moderation queue. Auth enforcement is done
// at the route-wiring layer (adminOnly wrapper) — this file is just
// business logic.
type CuratedShortsHandler struct {
	repo    *repository.CuratedShortRepo
	curator *curatedshorts.Curator
}

func NewCuratedShortsHandler(repo *repository.CuratedShortRepo, curator *curatedshorts.Curator) *CuratedShortsHandler {
	return &CuratedShortsHandler{repo: repo, curator: curator}
}

// Feed handles GET /api/v1/shorts/curated. Public; returns approved
// shorts. Query params:
//   - category: one of the configured slugs; empty = all categories
//   - limit: max rows, capped at 100
//   - offset: pagination
func (h *CuratedShortsHandler) Feed(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	if category != "" && curatedshorts.CategoryBySlug(category) == nil {
		api.Error(w, http.StatusBadRequest, "unknown category")
		return
	}
	limit := parseIntQuery(r, "limit", 30)
	offset := parseIntQuery(r, "offset", 0)

	shorts, err := h.repo.ListFeed(r.Context(), category, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list curated shorts")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"data":   shorts,
		"limit":  limit,
		"offset": offset,
	})
}

// Categories handles GET /api/v1/shorts/curated/categories. Returns
// the five (or whatever) configured category slugs + display names so
// the frontend can render filter tabs without hardcoding the list.
func (h *CuratedShortsHandler) Categories(w http.ResponseWriter, r *http.Request) {
	type cat struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
	}
	out := make([]cat, 0, len(curatedshorts.Categories))
	for _, c := range curatedshorts.Categories {
		out = append(out, cat{Slug: c.Slug, DisplayName: c.DisplayName})
	}
	api.JSON(w, http.StatusOK, out)
}

// --- Admin-only endpoints below. Auth handled by the adminOnly
// wrapper in routes.go — these methods assume the caller is already
// authenticated as an admin. ---

// Refresh handles POST /api/v1/admin/shorts/refresh.
//
// Async — fire-and-forget. With ~250 candidate videos × ~1-2s per
// LLM round-trip, a synchronous refresh takes 4-8 minutes, well
// past Cloudflare's 100s gateway timeout. We respond immediately
// with {status: "started"} and run the curator on a detached
// goroutine. Operator polls GET /admin/shorts/pending to see the
// queue populate as work completes.
func (h *CuratedShortsHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	go func() {
		// Detach from the request context — the HTTP response has
		// already returned by the time this runs, so r.Context() is
		// cancelled. 15-minute ceiling is plenty for the worst case
		// 250-video × 2s pass and stops a wedged LLM endpoint from
		// pinning a goroutine forever.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = h.curator.Refresh(ctx)
	}()
	api.JSON(w, http.StatusAccepted, map[string]any{
		"status":  "started",
		"message": "Refresh running in background. Poll GET /api/v1/admin/shorts/pending to watch the queue fill up.",
	})
}

// Pending handles GET /api/v1/admin/shorts/pending — moderation queue
// ordered best-score first.
func (h *CuratedShortsHandler) Pending(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 50)
	shorts, err := h.repo.ListPending(r.Context(), limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list pending")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": shorts})
}

// Approve handles POST /api/v1/admin/shorts/{id}/approve.
func (h *CuratedShortsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "approved")
}

// Reject handles POST /api/v1/admin/shorts/{id}/reject.
func (h *CuratedShortsHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "rejected")
}

// PurgePending handles POST /api/v1/admin/shorts/purge-pending.
// One-shot bulk reject of every currently-pending row — used after
// scoring-rule changes invalidate the existing queue.
func (h *CuratedShortsHandler) PurgePending(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	n, err := h.repo.RejectAllPending(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "purge failed")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"rejected": n})
}

func (h *CuratedShortsHandler) setStatus(w http.ResponseWriter, r *http.Request, status string) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id required")
		return
	}
	if err := h.repo.SetStatus(r.Context(), id, status, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to set status")
		return
	}
	s, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "short not found")
		return
	}
	api.JSON(w, http.StatusOK, s)
}
