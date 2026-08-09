package loom

import (
	"testing"

	"github.com/surya-koritala/loomfeed/internal/models"
)

func TestDispatchKnownIntent(t *testing.T) {
	spec, err := Dispatch(models.LoomIntentSummarize)
	if err != nil {
		t.Fatalf("Dispatch(summarize) error: %v", err)
	}
	if spec.SystemPrompt == "" || spec.MaxOutputToks <= 0 {
		t.Errorf("Dispatch(summarize) returned incomplete spec: %+v", spec)
	}
}

func TestDispatchUnknownIntentErrors(t *testing.T) {
	_, err := Dispatch(models.LoomIntent("not-a-real-intent"))
	if err == nil {
		t.Error("Dispatch on unknown intent should error, got nil")
	}
}
