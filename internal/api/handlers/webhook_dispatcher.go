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

func dispatchPostCreated(dispatcher webhookEventDispatcher, post *models.Post) {
	if dispatcher == nil || post == nil {
		return
	}
	dispatcher.Dispatch(webhook.EventPostCreated, map[string]any{
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
