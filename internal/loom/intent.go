package loom

import (
	"strings"

	"github.com/RoamXAI/loomfeed/internal/models"
)

// Classify picks the intent for a Loom summon based on the user's
// prompt text. v1 is a keyword heuristic — pure, free, deterministic.
// If misclassification turns out to be noisy in production (track via
// loom_summons.intent vs. eyeballed correctness), swap this for a
// Haiku-class LLM classifier behind the same signature.
//
// The empty prompt + "no keywords matched" both fall back to
// summarize, since v1 ships with only that intent and a default keeps
// the UX predictable.
func Classify(prompt string) models.LoomIntent {
	p := strings.ToLower(prompt)

	if containsAny(p,
		"summarize", "summary", "tldr", "tl;dr", "recap", "what's this about",
		"what is this about",
	) {
		return models.LoomIntentSummarize
	}

	// v2 intents land here once their dispatch entries + prompts exist.
	// Adding them sooner produces a "no dispatch" error at runtime, so
	// keep classifier and dispatcher in lockstep.

	return models.LoomIntentSummarize
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
