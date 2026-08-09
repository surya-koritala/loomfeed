package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/linkpreview"
)

type LinkPreviewHandler struct{}

func NewLinkPreviewHandler() *LinkPreviewHandler {
	return &LinkPreviewHandler{}
}

func (h *LinkPreviewHandler) Fetch(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		api.Error(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	// Return 200 with an empty preview when the target site blocks
	// scraping, times out, or returns a non-HTML page. This is not an
	// error from the caller's perspective — missing OG tags are normal
	// and the frontend renders a fallback. Returning 502 produces
	// console noise on every feed render that contains such a link.
	preview, err := linkpreview.Fetch(url)
	if err != nil {
		api.JSON(w, http.StatusOK, map[string]any{"url": url})
		return
	}

	api.JSON(w, http.StatusOK, preview)
}

// FetchVideo handles GET /api/v1/embed/video-extract?url=X.
//
// Scrapes the given source page for inline video URLs (og:video,
// twitter:player, <video src>, <source src>, YouTube/Vimeo iframes).
// Returns the first one found; null when the page has no inline
// video. Powers the post-detail "render video inline when the source
// has one but our ingestion didn't capture it" affordance — see
// docs/CYNTR_AGENT_CONTENT_ENGINE.md (Bonus section) for the
// scrape-during-ingestion plan that eventually replaces this.
//
// Cache-Control is set to 1h public so the same source URL doesn't
// get re-scraped on every viewer's first paint.
func (h *LinkPreviewHandler) FetchVideo(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		api.Error(w, http.StatusBadRequest, "url parameter is required")
		return
	}

	video, err := linkpreview.FetchVideo(url)
	w.Header().Set("Cache-Control", "public, s-maxage=3600, stale-while-revalidate=86400")
	if err != nil {
		// Same posture as link preview — return 200 with a null video
		// when scraping fails. Frontend treats nil as "no inline video,
		// fall back to cover image."
		api.JSON(w, http.StatusOK, map[string]any{"video": nil})
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"video": video})
}
