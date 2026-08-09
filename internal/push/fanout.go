package push

import (
	"context"
	"errors"

	"github.com/RoamXAI/loomfeed/internal/repository"
)

// Fanout glues NotificationRepo.Create to the Web Push sender.
// Implements repository.PushFanout.
type Fanout struct {
	sender *Sender
	subs   *repository.PushSubscriptionRepo
}

func NewFanout(sender *Sender, subs *repository.PushSubscriptionRepo) *Fanout {
	return &Fanout{sender: sender, subs: subs}
}

// Notify looks up every active subscription for a recipient and ships
// a push to each. Purges any endpoint that comes back gone (410/404).
// Silent no-op when the sender is unconfigured.
func (f *Fanout) Notify(ctx context.Context, recipientID, notifType, message string, postID *string) {
	if f == nil || f.sender == nil || !f.sender.Enabled() {
		return
	}
	subs, err := f.subs.ListByParticipant(ctx, recipientID)
	if err != nil || len(subs) == 0 {
		return
	}
	url := "/notifications"
	if postID != nil && *postID != "" {
		url = "/post/" + *postID
	}
	n := Notification{
		Title: titleFor(notifType),
		Body:  message,
		URL:   url,
		Tag:   notifType,
	}
	for _, s := range subs {
		err := f.sender.Send(ctx, Subscription{
			Endpoint:  s.Endpoint,
			P256dhKey: s.P256dhKey,
			AuthKey:   s.AuthKey,
		}, n)
		if errors.Is(err, ErrGone) {
			_ = f.subs.DeleteByEndpoint(ctx, s.Endpoint)
		}
	}
}

// titleFor turns a notification type into a short push title.
func titleFor(t string) string {
	switch t {
	case "mention":
		return "You were mentioned"
	case "post_comment":
		return "New reply"
	case "post_reaction", "comment_reaction":
		return "New reaction"
	case "vote":
		return "Someone voted on your post"
	case "arena_battle":
		return "Arena update"
	case "follow":
		return "New follower"
	default:
		return "Loomfeed"
	}
}
