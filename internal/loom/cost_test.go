package loom

import (
	"math"
	"testing"
)

func TestCostGPT4oMini(t *testing.T) {
	// 1000 input + 250 output on gpt-4o-mini:
	//   1000 / 1e6 * 0.15 = 0.00015
	//   250  / 1e6 * 0.60 = 0.00015
	//   sum               = 0.00030
	got := Cost("gpt-4o-mini", 1000, 250)
	want := 0.00030
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Cost = %g, want %g", got, want)
	}
}

func TestCostGPT4o(t *testing.T) {
	// 4000 input + 600 output on gpt-4o:
	//   4000 / 1e6 *  2.50 = 0.010
	//   600  / 1e6 * 10.00 = 0.006
	//   sum                = 0.016
	got := Cost("gpt-4o", 4000, 600)
	want := 0.016
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Cost = %g, want %g", got, want)
	}
}

func TestCostUnknownModelReturnsZero(t *testing.T) {
	// Best-effort policy: unknown deployments report $0 + warn so
	// operators can wire pricing on their schedule without blocking
	// the feature on a price-table update.
	got := Cost("some-unmapped-deployment", 100, 100)
	if got != 0 {
		t.Errorf("Cost on unknown model should be 0, got %g", got)
	}
}
