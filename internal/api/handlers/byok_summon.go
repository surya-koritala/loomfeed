package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	cryptopkg "github.com/RoamXAI/loomfeed/internal/crypto"
	"github.com/RoamXAI/loomfeed/internal/llm"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// BYOKSummonHandler lets a human user ask one of their BYOK agents to reply
// to a specific post. The server loads the post, decrypts the agent's API
// key, calls the configured LLM with the agent's persona + post context,
// and creates a comment authored by the agent participant.
type BYOKSummonHandler struct {
	pool     *pgxpool.Pool
	byok     *repository.BYOKAgentRepo
	posts    *repository.PostRepo
	comments *repository.CommentRepo
	vault    *cryptopkg.BYOKVault
}

func NewBYOKSummonHandler(pool *pgxpool.Pool, byok *repository.BYOKAgentRepo, posts *repository.PostRepo, comments *repository.CommentRepo, vault *cryptopkg.BYOKVault) *BYOKSummonHandler {
	return &BYOKSummonHandler{pool: pool, byok: byok, posts: posts, comments: comments, vault: vault}
}

// Summon handles POST /api/v1/posts/{id}/summon.
// Body: {byok_agent_id: "..."}.
func (h *BYOKSummonHandler) Summon(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	var req struct {
		BYOKAgentID string `json:"byok_agent_id"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BYOKAgentID == "" {
		api.Error(w, http.StatusBadRequest, "byok_agent_id is required")
		return
	}

	agent, err := h.byok.GetByID(r.Context(), req.BYOKAgentID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "byok agent not found")
		return
	}
	if !agent.Enabled {
		api.Error(w, http.StatusBadRequest, "agent is disabled")
		return
	}

	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	apiKey, err := h.vault.Open(agent.EncryptedAPIKey)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to decrypt api key", err)
		return
	}

	provider, err := llm.New(llm.Config{
		Provider: agent.Provider,
		Model:    agent.Model,
		APIKey:   apiKey,
	})
	if err != nil {
		api.ErrorWithDetail(w, http.StatusBadRequest, "provider unsupported", err)
		return
	}

	systemPrompt := agent.PersonaPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = "You are a thoughtful commenter on loomfeed, a social platform where humans and AI agents share ideas. Reply to the post below concisely and substantively. Avoid sycophancy; push back where warranted."
	}

	userMessage := buildSummonPrompt(post.Title, post.Body)

	ctx, cancel := context.WithTimeout(r.Context(), 75_000_000_000) // 75s
	defer cancel()

	reply, err := provider.Generate(ctx, systemPrompt, userMessage)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusBadGateway, "llm call failed", err)
		return
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		api.Error(w, http.StatusBadGateway, "llm returned empty reply")
		return
	}

	comment := &models.Comment{
		PostID:     postID,
		AuthorID:   agent.AgentParticipantID,
		AuthorType: models.ParticipantAgent,
		Body:       reply,
	}
	created, err := h.comments.Create(r.Context(), comment)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create comment", err)
		return
	}

	api.JSON(w, http.StatusCreated, map[string]any{
		"comment_id": created.ID,
		"body":       created.Body,
	})
}

// buildSummonPrompt turns a post into the user-side message we send to the
// LLM. Kept short to keep token cost down; the persona carries the voice.
func buildSummonPrompt(title, body string) string {
	body = strings.TrimSpace(body)
	if len(body) > 6000 {
		body = body[:6000] + "…"
	}
	return fmt.Sprintf("Post title: %s\n\nPost body:\n%s\n\nWrite your reply as a comment. Plain markdown only.",
		title, body)
}
