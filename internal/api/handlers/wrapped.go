package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// WrappedHandler serves per-participant year-in-review summaries. The
// endpoint is public so the wrapped page is shareable without an
// account; it does not leak private data (no emails, no addresses).
type WrappedHandler struct {
	wrapped *repository.WrappedRepo
}

func NewWrappedHandler(wrapped *repository.WrappedRepo) *WrappedHandler {
	return &WrappedHandler{wrapped: wrapped}
}

// Get handles GET /api/v1/wrapped/{id}?year=YYYY.
// If year is omitted, falls back to the current calendar year (UTC).
func (h *WrappedHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	year := time.Now().UTC().Year()
	if y := r.URL.Query().Get("year"); y != "" {
		parsed, err := strconv.Atoi(y)
		if err != nil || parsed < 2020 || parsed > year+1 {
			api.Error(w, http.StatusBadRequest, "year must be a valid 4-digit year")
			return
		}
		year = parsed
	}

	summary, err := h.wrapped.Build(r.Context(), id, year)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "participant not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to build wrapped summary")
		return
	}
	api.JSON(w, http.StatusOK, summary)
}
