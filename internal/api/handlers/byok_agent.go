package handlers

import (
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	cryptopkg "github.com/surya-koritala/loomfeed/internal/crypto"
	"github.com/surya-koritala/loomfeed/internal/llm"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// BYOKAgentHandler manages the bring-your-own-key agent configs that let a
// human user create their own AI agent on the platform by supplying an API
// key for OpenAI/Anthropic/Google. The key is encrypted at rest.
type BYOKAgentHandler struct {
	pool         *pgxpool.Pool
	byok         *repository.BYOKAgentRepo
	participants *repository.ParticipantRepo
	vault        *cryptopkg.BYOKVault
}

func NewBYOKAgentHandler(pool *pgxpool.Pool, byok *repository.BYOKAgentRepo, participants *repository.ParticipantRepo, vault *cryptopkg.BYOKVault) *BYOKAgentHandler {
	return &BYOKAgentHandler{pool: pool, byok: byok, participants: participants, vault: vault}
}

// Create handles POST /api/v1/byok-agents.
// Body: {display_name, provider, model, api_key, persona_prompt}.
//
// Flow: create a new agent participant owned by the current user, encrypt
// the API key with the server-side KEK, persist the byok_agents row.
func (h *BYOKAgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.vault == nil {
		api.Error(w, http.StatusServiceUnavailable, "BYOK agents are not available on this server")
		return
	}
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.ParticipantType != string(models.ParticipantHuman) {
		api.Error(w, http.StatusForbidden, "only human users can create BYOK agents")
		return
	}

	var req struct {
		DisplayName   string `json:"display_name"`
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		APIKey        string `json:"api_key"`
		PersonaPrompt string `json:"persona_prompt"`
		Bio           string `json:"bio"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Model = strings.TrimSpace(req.Model)
	req.APIKey = strings.TrimSpace(req.APIKey)

	if req.DisplayName == "" {
		api.Error(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if !slices.Contains(llm.SupportedProviders(), req.Provider) {
		api.Error(w, http.StatusBadRequest, "provider must be one of: "+strings.Join(llm.SupportedProviders(), ", "))
		return
	}
	if req.APIKey == "" {
		api.Error(w, http.StatusBadRequest, "api_key is required")
		return
	}

	encrypted, err := h.vault.Seal(req.APIKey)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to encrypt api key", err)
		return
	}

	// Create an agent participant + agent_identity, owned by the caller.
	agent, err := h.participants.CreateAgent(r.Context(), &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: req.DisplayName,
			Bio:         req.Bio,
			TrustScore:  10, // same starter value as other new agents
		},
		OwnerID:       claims.ParticipantID,
		ModelProvider: req.Provider,
		ModelName:     req.Model,
		Capabilities:  []string{"chat"},
		MaxRPM:        30,
		ProtocolType:  models.ProtocolREST,
	})
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create agent", err)
		return
	}

	byok, err := h.byok.Create(r.Context(), repository.CreateBYOKInput{
		OwnerID:            claims.ParticipantID,
		AgentParticipantID: agent.ID,
		Provider:           req.Provider,
		Model:              req.Model,
		EncryptedAPIKey:    encrypted,
		PersonaPrompt:      req.PersonaPrompt,
	})
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save byok config", err)
		return
	}
	byok.DisplayName = agent.DisplayName

	api.JSON(w, http.StatusCreated, byok)
}

// List handles GET /api/v1/byok-agents — returns the caller's agents.
func (h *BYOKAgentHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	rows, err := h.byok.ListByOwner(r.Context(), claims.ParticipantID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list byok agents", err)
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"agents": rows})
}

// Delete handles DELETE /api/v1/byok-agents/{id}.
func (h *BYOKAgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.vault == nil {
		api.Error(w, http.StatusServiceUnavailable, "BYOK agents are not available on this server")
		return
	}
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.byok.Delete(r.Context(), id, claims.ParticipantID); err != nil {
		api.ErrorWithDetail(w, http.StatusNotFound, "not found", err)
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
