package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/webhook"
)

// WebhookHandler handles webhook registration and management.
type WebhookHandler struct {
	webhooks   *repository.WebhookRepo
	dispatcher *webhook.Dispatcher
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(webhooks *repository.WebhookRepo, dispatcher *webhook.Dispatcher) *WebhookHandler {
	return &WebhookHandler{
		webhooks:   webhooks,
		dispatcher: dispatcher,
	}
}

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// Create handles POST /api/v1/webhooks.
func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createWebhookRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.URL == "" {
		api.Error(w, http.StatusBadRequest, "url is required")
		return
	}

	if err := api.ValidateWebhookURL(req.URL); err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Secret == "" {
		api.Error(w, http.StatusBadRequest, "secret is required")
		return
	}
	if len(req.Events) == 0 {
		api.Error(w, http.StatusBadRequest, "at least one event is required")
		return
	}

	validEvents := map[string]bool{
		"post.created":    true,
		"comment.created": true,
		"mention":         true,
		"vote.received":   true,
		"answer.accepted": true,
	}
	for _, ev := range req.Events {
		if !validEvents[ev] {
			api.Error(w, http.StatusBadRequest, "invalid event type: "+ev)
			return
		}
	}

	hook, err := h.webhooks.Create(r.Context(), claims.ParticipantID, req.URL, req.Secret, req.Events)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}

	api.JSON(w, http.StatusCreated, hook)
}

// List handles GET /api/v1/webhooks.
func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	hooks, err := h.webhooks.ListByParticipant(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}

	if hooks == nil {
		hooks = []repository.Webhook{}
	}
	api.JSON(w, http.StatusOK, hooks)
}

// Delete handles DELETE /api/v1/webhooks/{id}.
func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	webhookID := r.PathValue("id")
	if webhookID == "" {
		api.Error(w, http.StatusBadRequest, "webhook id is required")
		return
	}

	if err := h.webhooks.Delete(r.Context(), webhookID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusNotFound, "webhook not found")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListDeliveries handles GET /api/v1/webhooks/{id}/deliveries.
func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	webhookID := r.PathValue("id")
	if webhookID == "" {
		api.Error(w, http.StatusBadRequest, "webhook id is required")
		return
	}

	// Verify ownership
	hook, err := h.webhooks.GetByID(r.Context(), webhookID)
	if err != nil || hook.ParticipantID != claims.ParticipantID {
		api.Error(w, http.StatusNotFound, "webhook not found")
		return
	}

	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	deliveries, err := h.webhooks.ListDeliveries(r.Context(), webhookID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list deliveries")
		return
	}

	if deliveries == nil {
		deliveries = []repository.WebhookDelivery{}
	}
	api.JSON(w, http.StatusOK, deliveries)
}

// Test handles POST /api/v1/webhooks/{id}/test.
func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	webhookID := r.PathValue("id")
	if webhookID == "" {
		api.Error(w, http.StatusBadRequest, "webhook id is required")
		return
	}

	// Verify ownership
	hook, err := h.webhooks.GetByID(r.Context(), webhookID)
	if err != nil || hook.ParticipantID != claims.ParticipantID {
		api.Error(w, http.StatusNotFound, "webhook not found")
		return
	}

	// Send a test event
	h.dispatcher.Dispatch("test", map[string]any{
		"webhook_id": webhookID,
		"message":    "This is a test event from loomfeed",
	})

	api.JSON(w, http.StatusOK, map[string]string{"status": "test event dispatched"})
}
