package loom

import (
	"fmt"

	"github.com/surya-koritala/loomfeed/internal/models"
)

// ModelSpec is what an intent dispatches to: the system prompt that
// shapes the Loom voice for this intent + the hard output cap that
// bounds cost-per-summon. The model itself is operator-controlled at
// the Manager level (the Azure deployment name from cfg.LLM), not
// per-intent — that's a v2 concern when fact-check / counter want
// different tiers.
type ModelSpec struct {
	SystemPrompt  string
	MaxOutputToks int
}

// dispatchTable maps intent → model spec. New intents land here.
//
// Output token caps are deliberately conservative; raising one
// directly raises the per-summon cost ceiling. Anything that needs
// retrieval or tool use will also gain a flag here once added.
var dispatchTable = map[models.LoomIntent]ModelSpec{
	models.LoomIntentSummarize: {
		SystemPrompt: promptSummarize,
		// 150 tokens ≈ 110 words — enough for "one sentence + 3
		// short bullets" without padding. Bumped down from 250 after
		// initial responses came back too long for the comment UI.
		MaxOutputToks: 150,
	},
}

// Dispatch returns the ModelSpec for an intent or an error if the
// intent has no entry. An unrecognised intent is a programmer bug
// (Classify produced something the dispatcher doesn't know about), so
// it fails loud rather than silently falling back.
func Dispatch(intent models.LoomIntent) (ModelSpec, error) {
	spec, ok := dispatchTable[intent]
	if !ok {
		return ModelSpec{}, fmt.Errorf("loom: no dispatch for intent %q", intent)
	}
	return spec, nil
}
