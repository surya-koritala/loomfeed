package loom

import "log/slog"

// modelPricing is per-million-token USD pricing per Azure OpenAI
// deployment name. Deployment names are operator-defined, so this
// table is the contract operators maintain when they wire up a new
// deployment. Unknown entries return $0 + a warning rather than an
// error: missing pricing should under-report cost (alertable from
// the Grafana panel) rather than block a feature ship.
//
// Keys here are the *deployment* names you configure in Azure OpenAI
// Studio. Common base models (gpt-4o, gpt-4o-mini) are seeded so
// out-of-the-box deployments named after their model just work.
// Operators who name deployments arbitrarily should add their entries
// when they roll the deployment out — same flow as updating the
// Grafana cost dashboard's panels.
type modelPrice struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

var modelPricing = map[string]modelPrice{
	// Azure OpenAI — deployments live on roamx-resource (resource
	// group roamGX). Update these when Microsoft changes prices.
	//
	// gpt-5.4-mini / gpt-5.4-nano: estimated pricing pending the
	// operator confirming the published Azure rates. Order-of-
	// magnitude correct so the cost dashboard isn't stuck at zero
	// at launch; treat as approximate until verified.
	"gpt-5.4-mini": {InputPerMTok: 0.25, OutputPerMTok: 1.00},
	"gpt-5.4-nano": {InputPerMTok: 0.10, OutputPerMTok: 0.40},

	// Common base-model deployments — keep for environments using
	// the conventional names rather than the roamx-resource naming.
	"gpt-4o":      {InputPerMTok: 2.50, OutputPerMTok: 10.00},
	"gpt-4o-mini": {InputPerMTok: 0.15, OutputPerMTok: 0.60},
}

// Cost returns the USD cost of a (model, input_tokens, output_tokens)
// triple. Unknown models return 0 + emit a slog.Warn so the operator
// notices missing pricing without the cost meter blocking inference.
// The Grafana panel for loom_inference_cost_usd_total surfaces the
// drift as a stuck-at-zero series.
func Cost(model string, inputTokens, outputTokens int) float64 {
	p, ok := modelPricing[model]
	if !ok {
		slog.Warn("loom: no pricing for model — reporting $0 cost",
			"model", model,
			"input_tokens", inputTokens,
			"output_tokens", outputTokens,
		)
		return 0
	}
	const mTok = 1_000_000.0
	return float64(inputTokens)/mTok*p.InputPerMTok +
		float64(outputTokens)/mTok*p.OutputPerMTok
}
