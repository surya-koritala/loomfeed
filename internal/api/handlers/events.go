package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/events"
)

// EventHandler handles SSE streaming endpoints.
type EventHandler struct {
	hub *events.Hub
	cfg *config.Config
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(hub *events.Hub, cfg *config.Config) *EventHandler {
	return &EventHandler{hub: hub, cfg: cfg}
}

// Stream handles GET /api/v1/events/stream
//
// Accepts auth from (in order): the lf_access cookie (preferred — set
// by Phase 1.2 cookie issuance, doesn't put the token in the URL),
// the Authorization header (Bearer), and the legacy ?token= query
// param. The query param is kept for backwards compatibility with
// older clients during the cookie soak; once those drain we'll drop
// it entirely (token in URL leaks via Referer + access logs).
func (h *EventHandler) Stream(w http.ResponseWriter, r *http.Request) {
	// Try to get claims from context first (set by middleware)
	claims := middleware.GetClaims(r.Context())

	if claims == nil {
		// Source priority: cookie → Authorization header → ?token= query.
		token := middleware.ReadAccessCookie(r)
		if token == "" {
			header := r.Header.Get("Authorization")
			token = strings.TrimPrefix(header, "Bearer ")
			if token == header {
				token = ""
			}
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != "" {
			if c, err := auth.ValidateToken(h.cfg.JWT.Secret, token); err == nil {
				// Mirror middleware.Auth's revocation check so a token
				// that was blacklisted (e.g. after password change or
				// explicit logout) can't open a new SSE stream.
				if middleware.TokenBlacklist != nil {
					if cached, _ := middleware.TokenBlacklist.Get(r.Context(), "token_blacklist:"+token); cached != nil {
						http.Error(w, `{"error":"token revoked"}`, http.StatusUnauthorized)
						return
					}
				}
				claims = c
			}
		}
	}

	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Use ResponseController for flushing (works through wrapped ResponseWriters)
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send a connected event immediately
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"participant_id\":\"%s\"}\n\n", claims.ParticipantID)
	_ = rc.Flush()

	ch := h.hub.Subscribe(claims.ParticipantID)
	defer h.hub.Unsubscribe(claims.ParticipantID, ch)

	notify := r.Context().Done()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			if err != nil {
				return
			}
			_ = rc.Flush()
		case <-notify:
			return
		}
	}
}

// StreamHealth is a simple endpoint to check if SSE is working without auth.
func (h *EventHandler) StreamHealth(w http.ResponseWriter, r *http.Request) {
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// PostStream handles GET /api/v1/events/post/{id} — live comment feed
// for a single post. Public (no auth required). We piggy-back on the
// participant-keyed hub by subscribing on the synthetic key
// "post:<id>", which comment.Create publishes to when a comment lands.
func (h *EventHandler) PostStream(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		http.Error(w, `{"error":"post id required"}`, http.StatusBadRequest)
		return
	}
	key := "post:" + postID

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"post_id\":\"%s\"}\n\n", postID)
	_ = rc.Flush()

	ch := h.hub.Subscribe(key)
	defer h.hub.Unsubscribe(key, ch)

	notify := r.Context().Done()
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			if err != nil {
				return
			}
			_ = rc.Flush()
		case <-notify:
			return
		}
	}
}
