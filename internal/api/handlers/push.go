package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// PushHandler exposes Web Push subscribe/unsubscribe plus a public
// endpoint to fetch the VAPID public key the frontend needs before
// calling pushManager.subscribe.
type PushHandler struct {
	subs *repository.PushSubscriptionRepo
	cfg  *config.Config
}

func NewPushHandler(subs *repository.PushSubscriptionRepo, cfg *config.Config) *PushHandler {
	return &PushHandler{subs: subs, cfg: cfg}
}

// PublicKey handles GET /api/v1/push/key. Returns the VAPID public
// key (empty string if push isn't configured on the server).
func (h *PushHandler) PublicKey(w http.ResponseWriter, r *http.Request) {
	api.JSON(w, http.StatusOK, map[string]any{
		"public_key": h.cfg.Push.PublicKey,
		"enabled":    h.cfg.Push.PublicKey != "" && h.cfg.Push.PrivateKey != "",
	})
}

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Subscribe handles POST /api/v1/push/subscribe. Body shape matches
// the output of PushSubscription.toJSON() so the client can just POST
// what the browser gave it.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid subscription body")
		return
	}
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		api.Error(w, http.StatusBadRequest, "endpoint, keys.p256dh, and keys.auth are required")
		return
	}
	ua := r.Header.Get("User-Agent")
	if err := h.subs.Upsert(r.Context(), claims.ParticipantID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, ua); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to save subscription")
		return
	}
	api.JSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// Unsubscribe handles POST /api/v1/push/unsubscribe.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req unsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" {
		api.Error(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	if err := h.subs.DeleteByEndpoint(r.Context(), req.Endpoint); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unsubscribe")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
