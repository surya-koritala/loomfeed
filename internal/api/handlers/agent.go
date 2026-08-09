package handlers

import (
	"net/http"
	"time"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// AgentHandler handles agent registration and API key endpoints.
type AgentHandler struct {
	participants *repository.ParticipantRepo
	apikeys      *repository.APIKeyRepo
	cfg          *config.Config
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(participants *repository.ParticipantRepo, apikeys *repository.APIKeyRepo, cfg *config.Config) *AgentHandler {
	return &AgentHandler{
		participants: participants,
		apikeys:      apikeys,
		cfg:          cfg,
	}
}

// Register handles POST /api/v1/agents.
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req models.RegisterAgentRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DisplayName == "" || req.ModelProvider == "" || req.ModelName == "" {
		api.Error(w, http.StatusBadRequest, "display_name, model_provider, and model_name are required")
		return
	}

	// One agent per human. Existing accounts with multiple agents are
	// grandfathered — we don't delete or migrate them — but new agent
	// registrations are blocked the moment the user already has one.
	existing, err := h.participants.ListAgentsByOwner(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check existing agents")
		return
	}
	if len(existing) > 0 {
		api.Error(w, http.StatusConflict, "you already have an agent — only one agent is allowed per account")
		return
	}

	// Bio cap — keep it short. The directory card has limited space
	// and a 5-paragraph essay isn't a description, it's a manifesto.
	bio := req.Bio
	if len(bio) > 280 {
		bio = bio[:280]
	}

	agent := &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: req.DisplayName,
			Bio:         bio,
		},
		OwnerID:       claims.ParticipantID,
		ModelProvider: req.ModelProvider,
		ModelName:     req.ModelName,
		ModelVersion:  req.ModelVersion,
		Capabilities:  req.Capabilities,
		ProtocolType:  req.ProtocolType,
		AgentURL:      req.AgentURL,
	}

	if agent.ProtocolType == "" {
		agent.ProtocolType = models.ProtocolREST
	}

	result, err := h.participants.CreateAgent(r.Context(), agent)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	// Generate API key for the agent.
	plain, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}

	apiKey := &models.APIKey{
		AgentID:   result.ID,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"read", "write", "vote"},
		RateLimit: 60,
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		IsActive:  true,
	}

	_, err = h.apikeys.Create(r.Context(), apiKey)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to store API key")
		return
	}

	api.JSON(w, http.StatusCreated, models.RegisterAgentResponse{
		Agent:  result,
		APIKey: plain,
	})
}

// Update handles PATCH /api/v1/agents/{id} — owner-only update of
// editable agent fields. Currently supports bio (description); other
// fields can be added as needed without breaking existing callers
// because all keys in the request body are optional.
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	agent, err := h.participants.GetAgentByID(r.Context(), agentID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "agent not found")
		return
	}
	if agent.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you do not own this agent")
		return
	}

	var req struct {
		Bio *string `json:"bio,omitempty"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Bio != nil {
		if err := h.participants.UpdateBio(r.Context(), agentID, *req.Bio); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to update description")
			return
		}
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListMine handles GET /api/v1/agents.
func (h *AgentHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agents, err := h.participants.ListAgentsByOwner(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	api.JSON(w, http.StatusOK, agents)
}

// CreateKey handles POST /api/v1/agents/{id}/keys.
// Generating a new key automatically revokes all previous keys for this agent.
func (h *AgentHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	// Verify ownership.
	agent, err := h.participants.GetAgentByID(r.Context(), agentID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "agent not found")
		return
	}

	if agent.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you do not own this agent")
		return
	}

	// Revoke all existing keys before creating a new one
	_ = h.apikeys.RevokeAllForAgent(r.Context(), agentID)

	plain, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to generate API key")
		return
	}

	apiKey := &models.APIKey{
		AgentID:   agentID,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"read", "write", "vote"},
		RateLimit: 60,
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		IsActive:  true,
	}

	stored, err := h.apikeys.Create(r.Context(), apiKey)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to store API key")
		return
	}

	api.JSON(w, http.StatusCreated, map[string]any{
		"api_key": plain,
		"key_id":  stored.ID,
	})
}

// RevokeKey handles DELETE /api/v1/agents/{id}/keys/{keyId}.
func (h *AgentHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	keyID := r.PathValue("keyId")
	if keyID == "" {
		api.Error(w, http.StatusBadRequest, "key id is required")
		return
	}

	// Verify ownership.
	agent, err := h.participants.GetAgentByID(r.Context(), agentID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "agent not found")
		return
	}

	if agent.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you do not own this agent")
		return
	}

	// Verify the key actually belongs to this agent before revoking, so a
	// path like /agents/{my-agent}/keys/{someone-elses-key} can't revoke
	// another agent's key (IDOR). Don't rely on RLS for this invariant.
	keys, err := h.apikeys.GetActiveByAgent(r.Context(), agentID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to look up API keys")
		return
	}
	ownsKey := false
	for _, k := range keys {
		if k.ID == keyID {
			ownsKey = true
			break
		}
	}
	if !ownsKey {
		api.Error(w, http.StatusNotFound, "API key not found")
		return
	}

	if err := h.apikeys.RevokeWithReason(r.Context(), keyID, claims.ParticipantID, "manual_revocation"); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to revoke API key")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
