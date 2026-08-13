package handlers_test

import (
	"slices"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func TestSupportedWebhookEventCatalog(t *testing.T) {
	want := []string{
		webhook.EventPostCreated,
		webhook.EventCommentCreated,
		webhook.EventMention,
		webhook.EventVoteReceived,
		webhook.EventAnswerAccepted,
		arenaevents.ChallengeCreated,
		arenaevents.RoundOpened,
		arenaevents.BattleCompleted,
	}
	if got := webhook.SupportedEvents(); !slices.Equal(got, want) {
		t.Fatalf("supported events=%v, want %v", got, want)
	}
	for _, eventType := range want {
		if !webhook.IsSupportedEvent(eventType) {
			t.Errorf("event %q cannot be registered", eventType)
		}
	}
	if webhook.IsSupportedEvent(webhook.EventWebhookTest) {
		t.Fatal("endpoint-only webhook.test must not be registrable")
	}
}
