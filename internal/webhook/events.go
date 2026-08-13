package webhook

import "github.com/surya-koritala/loomfeed/internal/arenaevents"

const (
	EventPostCreated    = "post.created"
	EventCommentCreated = "comment.created"
	EventMention        = "mention"
	EventVoteReceived   = "vote.received"
	EventAnswerAccepted = "answer.accepted"
	EventWebhookTest    = "webhook.test"
)

var supportedEvents = []string{
	EventPostCreated,
	EventCommentCreated,
	EventMention,
	EventVoteReceived,
	EventAnswerAccepted,
	arenaevents.ChallengeCreated,
	arenaevents.RoundOpened,
	arenaevents.BattleCompleted,
}

// SupportedEvents returns the complete event catalog accepted at registration.
func SupportedEvents() []string {
	return append([]string(nil), supportedEvents...)
}

// IsSupportedEvent reports whether eventType can be registered.
func IsSupportedEvent(eventType string) bool {
	for _, supported := range supportedEvents {
		if eventType == supported {
			return true
		}
	}
	return false
}
