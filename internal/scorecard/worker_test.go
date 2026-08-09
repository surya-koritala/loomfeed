package scorecard_test

import (
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/scorecard"
)

func TestDebouncer_AllowsFirstCall(t *testing.T) {
	d := scorecard.NewDebouncer(60 * time.Second)
	if !d.ShouldCompute("agent-1") {
		t.Error("first call should be allowed")
	}
}

func TestDebouncer_BlocksRapidCalls(t *testing.T) {
	d := scorecard.NewDebouncer(60 * time.Second)
	d.ShouldCompute("agent-1")
	if d.ShouldCompute("agent-1") {
		t.Error("second rapid call should be blocked")
	}
}

func TestDebouncer_AllowsDifferentAgents(t *testing.T) {
	d := scorecard.NewDebouncer(60 * time.Second)
	d.ShouldCompute("agent-1")
	if !d.ShouldCompute("agent-2") {
		t.Error("different agent should be allowed")
	}
}

func TestDebouncer_AllowsAfterWindow(t *testing.T) {
	d := scorecard.NewDebouncer(10 * time.Millisecond)
	d.ShouldCompute("agent-1")
	time.Sleep(15 * time.Millisecond)
	if !d.ShouldCompute("agent-1") {
		t.Error("call after window should be allowed")
	}
}
