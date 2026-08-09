package loom

import (
	"testing"

	"github.com/surya-koritala/loomfeed/internal/models"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name, prompt string
		want         models.LoomIntent
	}{
		{"explicit summarize", "@loom summarize this", models.LoomIntentSummarize},
		{"tldr keyword", "tldr please", models.LoomIntentSummarize},
		{"tl;dr with semicolon", "tl;dr?", models.LoomIntentSummarize},
		{"recap variant", "give me a recap", models.LoomIntentSummarize},
		{"question form", "what is this about?", models.LoomIntentSummarize},
		{"mixed case", "TLDR THIS THREAD", models.LoomIntentSummarize},
		{"unknown defaults to summarize", "anything else", models.LoomIntentSummarize},
		{"empty prompt", "", models.LoomIntentSummarize},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.prompt)
			if got != c.want {
				t.Errorf("Classify(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}
