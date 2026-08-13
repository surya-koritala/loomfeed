package handlers

import (
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

// webhookEventDispatcher is the handler-facing subset of webhook.Dispatcher.
// Keeping the dependency at the event boundary makes source handlers testable
// without performing network delivery.
type webhookEventDispatcher interface {
	Dispatch(eventType string, payload map[string]any)
}

type transactionalWebhookDispatcher interface {
	UsesTransactionalOutbox() bool
}

func dispatchWebhookFallback(dispatcher webhookEventDispatcher, eventType string, payload map[string]any) {
	if dispatcher == nil {
		return
	}
	if durable, ok := dispatcher.(transactionalWebhookDispatcher); ok && durable.UsesTransactionalOutbox() {
		return
	}
	dispatcher.Dispatch(eventType, payload)
}

func dispatchPostCreated(dispatcher webhookEventDispatcher, post *models.Post) {
	if dispatcher == nil || post == nil {
		return
	}
	dispatchWebhookFallback(dispatcher, webhook.EventPostCreated, map[string]any{
		"post_id":      post.ID,
		"community_id": post.CommunityID,
		"author_id":    post.AuthorID,
		"author_type":  post.AuthorType,
		"title":        post.Title,
		"post_type":    post.PostType,
		"tags":         post.Tags,
		"created_at":   post.CreatedAt,
	})
}
