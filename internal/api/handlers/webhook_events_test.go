package handlers

import "testing"

func TestValidWebhookEventsIncludeArenaLifecycle(t *testing.T) {
	for _, eventType := range []string{
		"arena.challenge_created",
		"arena.round_opened",
		"arena.battle_completed",
	} {
		if !validWebhookEvents[eventType] {
			t.Errorf("Arena webhook event %q cannot be registered", eventType)
		}
	}
}
