package activitypub

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Publisher is the concrete APFanout the post handler calls after a
// successful create. It materializes the post as a Create{Note}
// activity and ships it to every follower's inbox.
type Publisher struct {
	store     *Store
	followers *FollowersRepo
	origin    string // canonical site URL, e.g. https://www.loomfeed.com
}

// NewPublisher wires the delivery pipeline. origin should be the same
// site URL the actor/outbox handlers use, with no trailing slash.
func NewPublisher(store *Store, followers *FollowersRepo, origin string) *Publisher {
	return &Publisher{
		store:     store,
		followers: followers,
		origin:    strings.TrimRight(origin, "/"),
	}
}

// Publish implements handlers.APFanout. Escapes content minimally and
// fans out via Deliver; per-follower failures are logged but don't
// propagate.
func (p *Publisher) Publish(ctx context.Context, authorID, postID, title, body string, createdAt time.Time) {
	actor, err := p.store.EnsureHandleAndKey(ctx, authorID)
	if err != nil {
		slog.Warn("ap: publisher load actor failed", "author_id", authorID, "err", err)
		return
	}
	priv, err := p.store.PrivateKeyPEM(ctx, authorID)
	if err != nil {
		slog.Warn("ap: publisher load key failed", "author_id", authorID, "err", err)
		return
	}

	actorURL := fmt.Sprintf("%s/users/%s", p.origin, actor.Handle)
	postURL := fmt.Sprintf("%s/post/%s", p.origin, postID)
	published := createdAt.UTC().Format("2006-01-02T15:04:05Z")

	note := map[string]any{
		"id":           postURL,
		"type":         "Note",
		"attributedTo": actorURL,
		"content":      fmt.Sprintf("<p><strong>%s</strong></p><p>%s</p>", escape(title), escape(body)),
		"published":    published,
		"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
		"cc":           []string{actorURL + "/followers"},
		"url":          postURL,
	}

	activity := map[string]any{
		"@context":  "https://www.w3.org/ns/activitystreams",
		"id":        fmt.Sprintf("%s/activity/%s", postURL, uuid.New().String()),
		"type":      "Create",
		"actor":     actorURL,
		"published": published,
		"to":        []string{"https://www.w3.org/ns/activitystreams#Public"},
		"cc":        []string{actorURL + "/followers"},
		"object":    note,
	}

	keyID := actorURL + "#main-key"
	FanoutCreate(ctx, p.followers, authorID, keyID, priv, activity)
}

// escape swaps the three characters that actually break HTML
// rendering in Mastodon's sanitizer. We deliberately don't strip
// markdown — downstream renderers do their own thing with the HTML.
func escape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}
