package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// MessageHandler handles direct messaging endpoints.
type MessageHandler struct {
	messages *repository.MessageRepo
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(messages *repository.MessageRepo) *MessageHandler {
	return &MessageHandler{messages: messages}
}

type sendMessageRequest struct {
	RecipientID string `json:"recipient_id"`
	Body        string `json:"body"`
}

// Send handles POST /api/v1/messages
// Auto-creates a conversation if one doesn't exist.
func (h *MessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req sendMessageRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RecipientID == "" {
		api.Error(w, http.StatusBadRequest, "recipient_id is required")
		return
	}
	if req.Body == "" {
		api.Error(w, http.StatusBadRequest, "body is required")
		return
	}
	if req.RecipientID == claims.ParticipantID {
		api.Error(w, http.StatusBadRequest, "cannot message yourself")
		return
	}

	// Find or create conversation
	convID, err := h.messages.CreateConversation(r.Context(), []string{claims.ParticipantID, req.RecipientID})
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}

	msg, err := h.messages.SendMessage(r.Context(), convID, claims.ParticipantID, req.Body)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	api.JSON(w, http.StatusCreated, msg)
}

// ListConversations handles GET /api/v1/messages/conversations
func (h *MessageHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	convs, err := h.messages.ListConversations(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}

	if convs == nil {
		convs = []repository.ConversationPreview{}
	}
	api.JSON(w, http.StatusOK, convs)
}

// GetConversation handles GET /api/v1/messages/conversations/{id}
func (h *MessageHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	convID := r.PathValue("id")
	if convID == "" {
		api.Error(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	// Verify participant is in the conversation
	ok, err := h.messages.IsParticipant(r.Context(), convID, claims.ParticipantID)
	if err != nil || !ok {
		api.Error(w, http.StatusNotFound, "conversation not found")
		return
	}

	limit := parseIntQuery(r, "limit", 50)
	offset := parseIntQuery(r, "offset", 0)

	msgs, err := h.messages.ListMessages(r.Context(), convID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list messages")
		return
	}

	if msgs == nil {
		msgs = []repository.Message{}
	}
	api.JSON(w, http.StatusOK, msgs)
}

// MarkRead handles PUT /api/v1/messages/conversations/{id}/read
func (h *MessageHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	convID := r.PathValue("id")
	if convID == "" {
		api.Error(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	if err := h.messages.MarkRead(r.Context(), convID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to mark as read")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "marked as read"})
}
