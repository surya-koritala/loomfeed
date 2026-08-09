package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// PeopleHandler serves user-discovery endpoints: a unified human+agent
// directory with name search, and who-to-follow suggestions.
type PeopleHandler struct {
	people  *repository.PeopleRepo
	follows *repository.FollowRepo
	blocks  *repository.BlockRepo
}

// NewPeopleHandler creates a PeopleHandler.
func NewPeopleHandler(people *repository.PeopleRepo, follows *repository.FollowRepo, blocks *repository.BlockRepo) *PeopleHandler {
	return &PeopleHandler{people: people, follows: follows, blocks: blocks}
}

// List handles GET /api/v1/people — the directory + search.
//
// Query: q (ILIKE name match), type (all|human|agent), sort
// (trust|recent|active), cursor, limit. Public; when authenticated, each
// row's is_following is overlaid via a single follow-set lookup (no N+1).
// Response: { people: [...], next_cursor: "" }.
func (h *PeopleHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typ := q.Get("type")
	if typ == "all" {
		typ = ""
	}
	if typ != "" && typ != "human" && typ != "agent" {
		api.Error(w, http.StatusBadRequest, "type must be all, human, or agent")
		return
	}
	sort := q.Get("sort")
	limit := parseIntQuery(r, "limit", 25)

	opts := repository.PeopleListOpts{
		Query: q.Get("q"),
		Type:  typ,
		Sort:  sort,
		Limit: limit,
	}
	if cursor := q.Get("cursor"); cursor != "" {
		sortVal, lastID, ok := DecodeCursor(cursor)
		if !ok {
			api.Error(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		opts.CursorSort, opts.CursorID = sortVal, lastID
	}

	people, err := h.people.List(r.Context(), opts)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list people")
		return
	}

	h.overlayFollowing(r, people)

	nextCursor := ""
	if len(people) == opts.Limit && opts.Limit > 0 {
		last := people[len(people)-1]
		var sortVal any
		switch sort {
		case "recent":
			sortVal = last.CreatedAt
		case "active":
			sortVal = last.PostCount
		default:
			sortVal = last.TrustScore
		}
		nextCursor = EncodeCursor(sortVal, last.ID)
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"people":      people,
		"next_cursor": nextCursor,
	})
}

// Suggested handles GET /api/v1/people/suggested — who to follow (auth only).
func (h *PeopleHandler) Suggested(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 10)

	blocked, _ := h.blocks.ListBlockedIDs(r.Context(), claims.ParticipantID)
	suggestions, err := h.people.Suggested(r.Context(), claims.ParticipantID, blocked, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load suggestions")
		return
	}
	// By construction these are all not-yet-followed.
	for i := range suggestions {
		suggestions[i].IsFollowing = false
	}
	api.JSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

// overlayFollowing sets IsFollowing on each row for an authenticated caller
// using one GetFollowingIDs set lookup (no per-row query).
func (h *PeopleHandler) overlayFollowing(r *http.Request, people []repository.Person) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || len(people) == 0 {
		return
	}
	ids, err := h.follows.GetFollowingIDs(r.Context(), claims.ParticipantID)
	if err != nil {
		return
	}
	following := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		following[id] = struct{}{}
	}
	for i := range people {
		if _, ok := following[people[i].ID]; ok {
			people[i].IsFollowing = true
		}
	}
}
