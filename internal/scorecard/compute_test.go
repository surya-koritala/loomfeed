package scorecard

import (
	"math"
	"testing"
)

func TestNormalizeLinear(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		max      float64
		expected float64
	}{
		{"zero value", 0, 100, 0},
		{"mid value", 50, 100, 0.5},
		{"max value", 100, 100, 1.0},
		{"over max clamps to 1", 150, 100, 1.0},
		{"negative clamps to 0", -10, 100, 0},
		{"zero max returns 0", 50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLinear(tt.value, tt.max)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("NormalizeLinear(%v, %v) = %v, want %v", tt.value, tt.max, got, tt.expected)
			}
		})
	}
}

func TestNormalizeLog(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		cap      float64
		expected float64
	}{
		{"zero value", 0, 500, 0}, // log2(1)/log2(501) ≈ 0
		{"cap value", 500, 500, 1.0},
		{"over cap clamps to 1", 1000, 500, 1.0},
		{"negative clamps to 0", -5, 500, 0},
		{"one post", 1, 500, math.Log2(2) / math.Log2(501)},
		{"zero cap returns 0", 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLog(tt.value, tt.cap)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("NormalizeLog(%v, %v) = %v, want %v", tt.value, tt.cap, got, tt.expected)
			}
		})
	}
}

func TestCompositeScore(t *testing.T) {
	// All signals present with known values.
	// We'll compute expected output manually.
	weights := DefaultWeights()
	signals := SignalScores{
		TrustScore:         Signal{Raw: 75, Normalized: 0.75, Weight: weights.TrustScore, HasData: true},
		Reputation:         Signal{Raw: 300, Normalized: 0.6, Weight: weights.Reputation, HasData: true},
		PredictionAccuracy: Signal{Raw: 0.8, Normalized: 0.8, Weight: weights.PredictionAccuracy, HasData: true},
		CorrectionRate:     Signal{Raw: 0.5, Normalized: 0.5, Weight: weights.CorrectionRate, HasData: true},
		ContentQuality:     Signal{Raw: 70, Normalized: 0.7, Weight: weights.ContentQuality, HasData: true},
		SourceReliability:  Signal{Raw: 0.65, Normalized: 0.65, Weight: weights.SourceReliability, HasData: true},
		PostCount:          Signal{Raw: 50, Normalized: NormalizeLog(50, 500), Weight: weights.PostCount, HasData: true},
		DomainExpertise:    Signal{Raw: 0.4, Normalized: 0.4, Weight: weights.DomainExpertise, HasData: true},
		Verification:       Signal{Raw: 1, Normalized: 1.0, Weight: weights.Verification, HasData: true},
		AcceptanceRate:     Signal{Raw: 0.55, Normalized: 0.55, Weight: weights.AcceptanceRate, HasData: true},
		Tenure:             Signal{Raw: 8, Normalized: 8.0 / 12.0, Weight: weights.Tenure, HasData: true},
	}

	got := ComputeComposite(signals, weights)

	// Manual calculation:
	// The original ten weights retain their relative proportions across 90%
	// of the score; correction rate owns the remaining 10%.
	postNorm := NormalizeLog(50, 500)
	expected := (0.75*0.135 + 0.60*0.09 + 0.80*0.18 + 0.50*0.10 + 0.70*0.135 +
		0.65*0.09 + postNorm*0.045 + 0.40*0.045 + 1.0*0.045 +
		0.55*0.09 + (8.0/12.0)*0.045) * 100

	if math.Abs(got-expected) > 0.01 {
		t.Errorf("ComputeComposite = %v, want ~%v", got, expected)
	}

	// Sanity: should be roughly 67
	if got < 60 || got > 75 {
		t.Errorf("ComputeComposite = %v, expected roughly in 60-75 range", got)
	}
}

func TestCompositeScore_CorrectionRateRaisesStanding(t *testing.T) {
	weights := DefaultWeights()
	ignored := makeUniformSignals(0.5, weights, true)
	ignored.CorrectionRate.Normalized = 0

	acknowledged := ignored
	acknowledged.CorrectionRate.Normalized = 1

	delta := ComputeComposite(acknowledged, weights) - ComputeComposite(ignored, weights)
	if math.Abs(delta-10) > 0.01 {
		t.Fatalf("owning every warranted correction changed score by %v, want 10", delta)
	}
}

func TestCompositeScore_MissingSignals(t *testing.T) {
	weights := DefaultWeights()

	// All signals present and normalized to 0.5 => score should be 50.
	allPresent := makeUniformSignals(0.5, weights, true)
	scoreAll := ComputeComposite(allPresent, weights)
	if math.Abs(scoreAll-50.0) > 0.01 {
		t.Errorf("all present uniform 0.5: got %v, want 50", scoreAll)
	}

	// Now mark epistemic (20%) and source reliability (10%) as missing.
	// Remaining weight = 0.70. Each signal's effective weight should be
	// original_weight / 0.70 so the sum still equals 1.0 (before *100).
	// All normalized = 0.5, so composite should still be 50.
	partial := makeUniformSignals(0.5, weights, true)
	partial.PredictionAccuracy.HasData = false
	partial.SourceReliability.HasData = false

	scorePartial := ComputeComposite(partial, weights)
	if math.Abs(scorePartial-50.0) > 0.01 {
		t.Errorf("missing signals uniform 0.5: got %v, want 50", scorePartial)
	}

	// All signals missing => score should be 0 (no data at all).
	noData := makeUniformSignals(0.5, weights, false)
	scoreNone := ComputeComposite(noData, weights)
	if scoreNone != 0 {
		t.Errorf("all missing: got %v, want 0", scoreNone)
	}
}

func TestTierFromScore(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{100, "elite"},
		{85, "elite"},
		{84.99, "trusted"},
		{65, "trusted"},
		{64.99, "rising"},
		{40, "rising"},
		{39.99, "new"},
		{0, "new"},
		{-5, "new"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := TierFromScore(tt.score)
			if got != tt.expected {
				t.Errorf("TierFromScore(%v) = %q, want %q", tt.score, got, tt.expected)
			}
		})
	}
}

func TestDefaultWeights_SumToOne(t *testing.T) {
	w := DefaultWeights()
	sum := w.TrustScore + w.Reputation + w.PredictionAccuracy + w.ContentQuality +
		w.SourceReliability + w.PostCount + w.DomainExpertise + w.Verification +
		w.AcceptanceRate + w.Tenure + w.CorrectionRate
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("DefaultWeights sum = %v, want 1.0", sum)
	}
	if w.CorrectionRate != 0.10 {
		t.Errorf("CorrectionRate weight = %v, want 0.10", w.CorrectionRate)
	}
}

// makeUniformSignals creates a SignalScores where every signal has the same
// normalized value and the given HasData flag.
func makeUniformSignals(norm float64, w Weights, hasData bool) SignalScores {
	return SignalScores{
		TrustScore:         Signal{Normalized: norm, Weight: w.TrustScore, HasData: hasData},
		Reputation:         Signal{Normalized: norm, Weight: w.Reputation, HasData: hasData},
		PredictionAccuracy: Signal{Normalized: norm, Weight: w.PredictionAccuracy, HasData: hasData},
		CorrectionRate:     Signal{Normalized: norm, Weight: w.CorrectionRate, HasData: hasData},
		ContentQuality:     Signal{Normalized: norm, Weight: w.ContentQuality, HasData: hasData},
		SourceReliability:  Signal{Normalized: norm, Weight: w.SourceReliability, HasData: hasData},
		PostCount:          Signal{Normalized: norm, Weight: w.PostCount, HasData: hasData},
		DomainExpertise:    Signal{Normalized: norm, Weight: w.DomainExpertise, HasData: hasData},
		Verification:       Signal{Normalized: norm, Weight: w.Verification, HasData: hasData},
		AcceptanceRate:     Signal{Normalized: norm, Weight: w.AcceptanceRate, HasData: hasData},
		Tenure:             Signal{Normalized: norm, Weight: w.Tenure, HasData: hasData},
	}
}
