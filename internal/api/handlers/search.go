package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// SearchHandler handles search endpoints.
type SearchHandler struct {
	search   *repository.SearchRepo
	hybrid   *repository.HybridSearchRepo
	suggest  *repository.SuggestRepo
	follows  *repository.FollowRepo
	embedder loom.Embedder
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(search *repository.SearchRepo, hybrid *repository.HybridSearchRepo) *SearchHandler {
	return &SearchHandler{search: search, hybrid: hybrid}
}

// WithSuggest wires the typeahead repo. Optional — if unset, /suggest
// returns empty groups.
func (h *SearchHandler) WithSuggest(s *repository.SuggestRepo) {
	h.suggest = s
}

// WithFollows wires the follow repo so search results can carry
// viewer_following for the in-context Subscribe CTA. Optional — if
// unset, the field stays false.
func (h *SearchHandler) WithFollows(f *repository.FollowRepo) {
	h.follows = f
}

// WithEmbedder enables semantic retrieval for hybrid search. It is optional:
// an unconfigured or unavailable provider leaves the lexical RRF path intact.
func (h *SearchHandler) WithEmbedder(e loom.Embedder) {
	h.embedder = e
}

// populateViewerFollowing marks results whose author the viewer
// follows — the same batch shape as FeedHandler.populateViewerFollowing,
// over pointers so both []PostWithAuthor and []SearchResult (which
// embeds PostWithAuthor) can share it. Best-effort: errors → no
// annotation, anonymous viewer → all false.
func (h *SearchHandler) populateViewerFollowing(ctx context.Context, posts []*models.PostWithAuthor, viewerID string) {
	if h.follows == nil || viewerID == "" || len(posts) == 0 {
		return
	}
	authorIDs := make([]string, 0, len(posts))
	seen := map[string]bool{}
	for _, p := range posts {
		if !seen[p.AuthorID] {
			seen[p.AuthorID] = true
			authorIDs = append(authorIDs, p.AuthorID)
		}
	}
	set, err := h.follows.FollowedSet(ctx, viewerID, authorIDs)
	if err != nil || len(set) == 0 {
		return
	}
	for _, p := range posts {
		p.ViewerFollowing = set[p.AuthorID]
	}
}

// Suggest handles GET /api/v1/search/suggest?q=...&limit=5.
// Lightweight typeahead for the nav bar: prefix-match against
// communities, participants, and post titles.
func (h *SearchHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit := min(parseIntQuery(r, "limit", 5), 10)
	if h.suggest == nil {
		api.JSON(w, http.StatusOK, map[string]any{
			"communities":  []any{},
			"participants": []any{},
			"posts":        []any{},
			"query":        query,
		})
		return
	}
	out, err := h.suggest.Suggest(r.Context(), query, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "suggest failed")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"communities":  out.Communities,
		"participants": out.Participants,
		"posts":        out.Posts,
		"query":        query,
	})
}

// Search handles GET /api/v1/search.
// Supports query params:
//   - q: search query (required)
//   - mode: "hybrid" (default) or "text" (legacy full-text only)
//   - limit: max results (default 25, max 100)
//   - offset: pagination offset (default 0)
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		api.Error(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := decodeCursorID(r.URL.Query().Get("cursor"))
	if cursor != "" {
		offset = 0
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "hybrid"
	}

	if mode != "hybrid" && mode != "text" {
		api.Error(w, http.StatusBadRequest, "mode must be 'hybrid' or 'text'")
		return
	}

	claims := middleware.GetClaims(r.Context())

	if mode == "text" {
		// Legacy full-text-only search
		results, total, err := h.search.SearchPosts(r.Context(), query, limit, offset, cursor)
		if err != nil {
			api.ErrorWithDetail(w, http.StatusInternalServerError, "search failed", err)
			return
		}
		if claims != nil {
			refs := make([]*models.PostWithAuthor, len(results))
			for i := range results {
				refs[i] = &results[i]
			}
			h.populateViewerFollowing(r.Context(), refs, claims.ParticipantID)
		}
		resp := models.PaginatedResponse{
			Data:        results,
			Total:       total,
			Limit:       limit,
			Offset:      offset,
			HasMore:     len(results) == limit,
			RetrievedAt: time.Now(),
		}
		if resp.HasMore && len(results) > 0 {
			last := results[len(results)-1]
			resp.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
		}
		api.JSON(w, http.StatusOK, resp)
		return
	}

	// Parse optional filter params
	filters := repository.SearchFilters{
		Community:  r.URL.Query().Get("community"),
		AuthorType: r.URL.Query().Get("author_type"),
		PostType:   r.URL.Query().Get("post_type"),
		Period:     r.URL.Query().Get("period"),
	}

	// Hybrid search (default). Query embedding is best-effort so provider
	// outages never turn an otherwise healthy lexical search into a 500.
	var queryEmbedding []float32
	if h.embedder != nil {
		embedCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		vector, embedErr := h.embedder.Embed(embedCtx, query)
		cancel()
		switch {
		case embedErr != nil:
			slog.Warn("hybrid search: query embedding failed; using lexical fallback", "error", embedErr)
		case len(vector) != repository.PostEmbeddingDimensions:
			slog.Warn(
				"hybrid search: query embedding dimension mismatch; using lexical fallback",
				"dimensions", len(vector),
				"expected", repository.PostEmbeddingDimensions,
			)
		default:
			queryEmbedding = vector
		}
	}

	var (
		results []models.SearchResult
		total   int
		err     error
	)
	if len(queryEmbedding) > 0 {
		results, total, err = h.hybrid.HybridSearchWithEmbedding(r.Context(), query, queryEmbedding, limit, offset, filters, cursor)
	} else {
		results, total, err = h.hybrid.HybridSearch(r.Context(), query, limit, offset, filters, cursor)
	}
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "search failed", err)
		return
	}

	if claims != nil {
		refs := make([]*models.PostWithAuthor, len(results))
		for i := range results {
			refs[i] = &results[i].PostWithAuthor
		}
		h.populateViewerFollowing(r.Context(), refs, claims.ParticipantID)
	}

	resp := models.SearchResponse{
		Data:        results,
		Total:       total,
		Query:       query,
		Mode:        mode,
		Limit:       limit,
		Offset:      offset,
		HasMore:     len(results) == limit,
		Community:   filters.Community,
		AuthorType:  filters.AuthorType,
		PostType:    filters.PostType,
		Period:      filters.Period,
		RetrievedAt: time.Now(),
	}
	if resp.HasMore && len(results) > 0 {
		last := results[len(results)-1]
		resp.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
	}
	api.JSON(w, http.StatusOK, resp)
}
