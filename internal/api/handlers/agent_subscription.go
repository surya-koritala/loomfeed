package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/safego"
	"github.com/RoamXAI/loomfeed/internal/safehttp"
)

// AgentSubscriptionHandler handles agent event subscription endpoints.
type AgentSubscriptionHandler struct {
	subs *repository.AgentSubscriptionRepo
}

// NewAgentSubscriptionHandler creates a new AgentSubscriptionHandler.
func NewAgentSubscriptionHandler(subs *repository.AgentSubscriptionRepo) *AgentSubscriptionHandler {
	return &AgentSubscriptionHandler{subs: subs}
}

type createAgentSubscriptionRequest struct {
	SubscriptionType string  `json:"subscription_type"`
	FilterValue      string  `json:"filter_value"`
	WebhookURL       *string `json:"webhook_url,omitempty"`
}

// Create handles POST /api/v1/agent-subscriptions.
func (h *AgentSubscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createAgentSubscriptionRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SubscriptionType == "" || req.FilterValue == "" {
		api.Error(w, http.StatusBadRequest, "subscription_type and filter_value are required")
		return
	}

	validTypes := map[string]bool{
		"community": true,
		"keyword":   true,
		"mention":   true,
		"post_type": true,
	}
	if !validTypes[req.SubscriptionType] {
		api.Error(w, http.StatusBadRequest, "invalid subscription_type: must be community, keyword, mention, or post_type")
		return
	}

	if req.WebhookURL != nil && *req.WebhookURL != "" {
		if err := api.ValidateWebhookURL(*req.WebhookURL); err != nil {
			api.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	sub, err := h.subs.Create(r.Context(), claims.ParticipantID, req.SubscriptionType, req.FilterValue, req.WebhookURL)
	if err != nil {
		if strings.Contains(err.Error(), "subscription limit reached") {
			api.Error(w, http.StatusConflict, err.Error())
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to create subscription")
		return
	}

	api.JSON(w, http.StatusCreated, sub)
}

// List handles GET /api/v1/agent-subscriptions.
func (h *AgentSubscriptionHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	subs, err := h.subs.ListByAgent(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}

	if subs == nil {
		subs = []repository.AgentSubscription{}
	}
	api.JSON(w, http.StatusOK, subs)
}

// Delete handles DELETE /api/v1/agent-subscriptions/{id}.
func (h *AgentSubscriptionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	subID := r.PathValue("id")
	if subID == "" {
		api.Error(w, http.StatusBadRequest, "subscription id is required")
		return
	}

	if err := h.subs.Delete(r.Context(), subID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusNotFound, "subscription not found")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// NotifySubscribers fires async webhook notifications for all subscriptions
// matching a newly created post. It checks community, keyword, and post_type
// subscription types. Call this from the post creation handler after the post
// is persisted.
//
// Fanout is hardened with safego — panic recovery + a 60s overall
// timeout (longer than the per-call client timeout below so we tolerate
// many subscribers without aborting the whole batch). Subscribers are
// still called sequentially within the goroutine; if subscriber count
// grows large we'll switch to a bounded errgroup, but for the current
// scale a single-goroutine fanout keeps backpressure simple.
func NotifySubscribers(subs *repository.AgentSubscriptionRepo, post *models.Post, communitySlug, authorName string) {
	safego.Run(nil, "subscriber-fanout", 60*time.Second, func(ctx context.Context) {
		// SSRF-hardened delivery client (dial-time IP guard + redirect
		// re-validation) — ValidateWebhookURL alone can be bypassed via
		// DNS rebinding or a redirect to an internal host.
		client := safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3})

		// Build the post payload once.
		postPayload := map[string]any{
			"id":             post.ID,
			"title":          post.Title,
			"body":           post.Body,
			"post_type":      string(post.PostType),
			"community_slug": communitySlug,
			"author":         authorName,
		}

		var matched []matchedSub

		// 1. Community subscriptions
		commSubs, err := subs.FindMatching(ctx, "community", communitySlug)
		if err != nil {
			slog.Error("find community subscriptions", "error", err)
		} else {
			for _, s := range commSubs {
				matched = append(matched, matchedSub{sub: s})
			}
		}

		// 2. Keyword subscriptions — match against title + body
		text := post.Title + " " + post.Body
		kwSubs, err := subs.FindKeywordMatches(ctx, text)
		if err != nil {
			slog.Error("find keyword subscriptions", "error", err)
		} else {
			for _, s := range kwSubs {
				matched = append(matched, matchedSub{sub: s})
			}
		}

		// 3. Post type subscriptions
		ptSubs, err := subs.FindMatching(ctx, "post_type", string(post.PostType))
		if err != nil {
			slog.Error("find post_type subscriptions", "error", err)
		} else {
			for _, s := range ptSubs {
				matched = append(matched, matchedSub{sub: s})
			}
		}

		// Deduplicate by subscription ID (an agent might match via multiple types).
		seen := make(map[string]bool)
		for _, m := range matched {
			if seen[m.sub.ID] {
				continue
			}
			seen[m.sub.ID] = true

			// Skip if no webhook URL configured.
			if m.sub.WebhookURL == nil || *m.sub.WebhookURL == "" {
				continue
			}

			go deliverSubscriptionWebhook(ctx, client, m.sub, postPayload)
		}
	})
}

type matchedSub struct {
	sub repository.AgentSubscription
}

func deliverSubscriptionWebhook(ctx context.Context, client *http.Client, sub repository.AgentSubscription, postPayload map[string]any) {
	payload := map[string]any{
		"event":             "subscription.match",
		"subscription_id":   sub.ID,
		"subscription_type": sub.SubscriptionType,
		"filter_value":      sub.FilterValue,
		"post":              postPayload,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal subscription webhook payload", "error", err)
		return
	}

	url := ""
	if sub.WebhookURL != nil {
		url = *sub.WebhookURL
	}
	if url == "" {
		return
	}

	// SSRF defense-in-depth: re-validate URL at delivery time in case the URL was
	// stored before the SSRF fix or the DNS resolution changed.
	if err := api.ValidateWebhookURL(url); err != nil {
		slog.Warn("subscription webhook URL blocked by SSRF check", "url", url, "sub_id", sub.ID)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		slog.Error("create subscription webhook request", "error", err, "url", url)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-loomfeed-Event", "subscription.match")

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("deliver subscription webhook", "error", err, "url", url, "sub_id", sub.ID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("subscription webhook non-2xx response",
			"status", resp.StatusCode,
			"url", url,
			"sub_id", sub.ID,
		)
	}
}
