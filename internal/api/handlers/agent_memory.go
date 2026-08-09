package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

const (
	maxKeyLength   = 256
	maxValueSize   = 64 * 1024 // 64KB
	maxKeysPerAgent = 1000
)

// AgentMemoryHandler handles agent memory CRUD operations.
type AgentMemoryHandler struct {
	memory *repository.AgentMemoryRepo
}

// NewAgentMemoryHandler creates a new AgentMemoryHandler.
func NewAgentMemoryHandler(memory *repository.AgentMemoryRepo) *AgentMemoryHandler {
	return &AgentMemoryHandler{memory: memory}
}

// Set handles PUT /api/v1/agent-memory/{key} — upsert a key-value pair.
func (h *AgentMemoryHandler) Set(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		api.Error(w, http.StatusBadRequest, "key is required")
		return
	}
	if len(key) > maxKeyLength {
		api.Error(w, http.StatusBadRequest, "key exceeds maximum length of 256 characters")
		return
	}

	// Read and validate value size
	body, err := io.ReadAll(io.LimitReader(r.Body, maxValueSize+1))
	if err != nil {
		api.Error(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if len(body) > maxValueSize {
		api.Error(w, http.StatusRequestEntityTooLarge, "value exceeds maximum size of 64KB")
		return
	}
	if len(body) == 0 {
		api.Error(w, http.StatusBadRequest, "request body is required")
		return
	}

	// Validate that body is valid JSON
	if !json.Valid(body) {
		api.Error(w, http.StatusBadRequest, "value must be valid JSON")
		return
	}

	// Check key limit: only enforce for new keys (not updates)
	existing, err := h.memory.Get(r.Context(), claims.ParticipantID, key)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check existing key")
		return
	}
	if existing == nil {
		count, err := h.memory.Count(r.Context(), claims.ParticipantID)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to check key count")
			return
		}
		if count >= maxKeysPerAgent {
			api.Error(w, http.StatusConflict, "maximum of 1000 keys per agent reached")
			return
		}
	}

	if err := h.memory.Set(r.Context(), claims.ParticipantID, key, json.RawMessage(body)); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to set memory")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "ok", "key": key})
}

// Get handles GET /api/v1/agent-memory/{key} — retrieve a single key.
func (h *AgentMemoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		api.Error(w, http.StatusBadRequest, "key is required")
		return
	}

	entry, err := h.memory.Get(r.Context(), claims.ParticipantID, key)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get memory")
		return
	}
	if entry == nil {
		api.Error(w, http.StatusNotFound, "key not found")
		return
	}

	api.JSON(w, http.StatusOK, entry)
}

// List handles GET /api/v1/agent-memory — list all keys, optionally filtered by prefix.
func (h *AgentMemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	prefix := r.URL.Query().Get("prefix")

	entries, err := h.memory.List(r.Context(), claims.ParticipantID, prefix)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list memory")
		return
	}

	if entries == nil {
		entries = []repository.AgentMemoryEntry{}
	}
	api.JSON(w, http.StatusOK, entries)
}

// Delete handles DELETE /api/v1/agent-memory/{key} — delete a single key.
func (h *AgentMemoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	key := r.PathValue("key")
	if key == "" {
		api.Error(w, http.StatusBadRequest, "key is required")
		return
	}

	if err := h.memory.Delete(r.Context(), claims.ParticipantID, key); err != nil {
		api.Error(w, http.StatusNotFound, "key not found")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// DeleteAll handles DELETE /api/v1/agent-memory — clear all memory for the agent.
func (h *AgentMemoryHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.memory.DeleteAll(r.Context(), claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to clear memory")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}
