package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// AgentCapabilityHandler handles agent capability discovery endpoints.
type AgentCapabilityHandler struct {
	caps *repository.AgentCapabilityRepo
}

// NewAgentCapabilityHandler creates a new AgentCapabilityHandler.
func NewAgentCapabilityHandler(caps *repository.AgentCapabilityRepo) *AgentCapabilityHandler {
	return &AgentCapabilityHandler{caps: caps}
}

type registerCapabilityRequest struct {
	Capability   string          `json:"capability"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
	EndpointURL  string          `json:"endpoint_url,omitempty"`
}

// Register handles POST /api/v1/agent-capabilities.
func (h *AgentCapabilityHandler) Register(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req registerCapabilityRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Capability = strings.TrimSpace(req.Capability)
	if req.Capability == "" {
		api.Error(w, http.StatusBadRequest, "capability is required")
		return
	}
	if len(req.Capability) > 100 {
		api.Error(w, http.StatusBadRequest, "capability must be 100 characters or fewer")
		return
	}

	// Validate endpoint URL if provided
	if req.EndpointURL != "" {
		if !strings.HasPrefix(req.EndpointURL, "http://") && !strings.HasPrefix(req.EndpointURL, "https://") {
			api.Error(w, http.StatusBadRequest, "endpoint_url must start with http:// or https://")
			return
		}
	}

	cap, err := h.caps.Register(r.Context(), claims.ParticipantID, req.Capability, req.Description, req.InputSchema, req.OutputSchema, req.EndpointURL)
	if err != nil {
		if strings.Contains(err.Error(), "capability limit reached") {
			api.Error(w, http.StatusConflict, err.Error())
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to register capability", err)
		return
	}

	api.JSON(w, http.StatusCreated, cap)
}

// Unregister handles DELETE /api/v1/agent-capabilities/{capability}.
func (h *AgentCapabilityHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	capability := r.PathValue("capability")
	if capability == "" {
		api.Error(w, http.StatusBadRequest, "capability is required")
		return
	}

	if err := h.caps.Unregister(r.Context(), claims.ParticipantID, capability); err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.Error(w, http.StatusNotFound, "capability not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to unregister capability", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListMine handles GET /api/v1/agent-capabilities.
func (h *AgentCapabilityHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	caps, err := h.caps.GetByAgent(r.Context(), claims.ParticipantID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list capabilities", err)
		return
	}

	if caps == nil {
		caps = []repository.AgentCapability{}
	}
	api.JSON(w, http.StatusOK, caps)
}

// Search handles GET /api/v1/discover — public discovery.
func (h *AgentCapabilityHandler) Search(w http.ResponseWriter, r *http.Request) {
	capability := r.URL.Query().Get("capability")
	limit := parseIntQuery(r, "limit", 20)
	offset := parseIntQuery(r, "offset", 0)

	if limit > 100 {
		limit = 100
	}

	minRating := 0.0
	if v := r.URL.Query().Get("min_rating"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			minRating = parsed
		}
	}

	verifiedOnly := false
	if v := r.URL.Query().Get("verified_only"); v == "true" || v == "1" {
		verifiedOnly = true
	}

	results, total, err := h.caps.Search(r.Context(), capability, minRating, verifiedOnly, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to search capabilities", err)
		return
	}

	if results == nil {
		results = []repository.AgentCapabilityWithAgent{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"agents": results,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// SearchByCapability handles GET /api/v1/discover/{capability}.
func (h *AgentCapabilityHandler) SearchByCapability(w http.ResponseWriter, r *http.Request) {
	capability := r.PathValue("capability")
	if capability == "" {
		api.Error(w, http.StatusBadRequest, "capability is required")
		return
	}

	limit := parseIntQuery(r, "limit", 20)
	offset := parseIntQuery(r, "offset", 0)

	if limit > 100 {
		limit = 100
	}

	minRating := 0.0
	if v := r.URL.Query().Get("min_rating"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			minRating = parsed
		}
	}

	verifiedOnly := false
	if v := r.URL.Query().Get("verified_only"); v == "true" || v == "1" {
		verifiedOnly = true
	}

	results, total, err := h.caps.Search(r.Context(), capability, minRating, verifiedOnly, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to search capabilities", err)
		return
	}

	if results == nil {
		results = []repository.AgentCapabilityWithAgent{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"agents": results,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Invoke handles POST /api/v1/discover/{id}/invoke.
// For now this increments usage and returns the capability info.
// Future: proxy to the agent's endpoint_url.
func (h *AgentCapabilityHandler) Invoke(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	capID := r.PathValue("id")
	if capID == "" {
		api.Error(w, http.StatusBadRequest, "capability id is required")
		return
	}

	// Look up the capability
	cap, err := h.caps.GetByID(r.Context(), capID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "capability not found")
		return
	}

	// Increment usage count
	_ = h.caps.IncrementUsage(r.Context(), capID)

	// Return capability info for the caller to invoke directly
	api.JSON(w, http.StatusOK, map[string]any{
		"capability":   cap,
		"endpoint_url": cap.EndpointURL,
		"message":      "Use the endpoint_url to invoke this capability directly",
	})
}

// RateCapability handles POST /api/v1/discover/{id}/rate.
func (h *AgentCapabilityHandler) RateCapability(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	capID := r.PathValue("id")
	if capID == "" {
		api.Error(w, http.StatusBadRequest, "capability id is required")
		return
	}

	var req struct {
		Rating float64 `json:"rating"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Rating < 0 || req.Rating > 5 {
		api.Error(w, http.StatusBadRequest, "rating must be between 0 and 5")
		return
	}

	if err := h.caps.Rate(r.Context(), capID, req.Rating); err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.Error(w, http.StatusNotFound, "capability not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to rate capability", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "rated"})
}
