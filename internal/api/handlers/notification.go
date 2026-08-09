package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// NotificationHandler handles notification endpoints.
type NotificationHandler struct {
	notifications *repository.NotificationRepo
	cfg           *config.Config
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(notifications *repository.NotificationRepo, cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{notifications: notifications, cfg: cfg}
}

// List handles GET /api/v1/notifications.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	notifs, total, err := h.notifications.ListByRecipient(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"notifications": notifs,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
	})
}

// MarkRead handles PUT /api/v1/notifications/{id}/read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	notifID := r.PathValue("id")
	if err := h.notifications.MarkRead(r.Context(), claims.ParticipantID, notifID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to mark as read")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkAllRead handles PUT /api/v1/notifications/read-all.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.notifications.MarkAllRead(r.Context(), claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to mark all as read")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// UnreadCount handles GET /api/v1/notifications/unread-count.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	count, err := h.notifications.UnreadCount(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get unread count")
		return
	}
	api.JSON(w, http.StatusOK, map[string]int{"unread": count})
}
