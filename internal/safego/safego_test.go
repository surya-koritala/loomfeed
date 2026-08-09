package safego

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRun_ExecutesCallback(t *testing.T) {
	var ran int32
	done := make(chan struct{})
	Run(context.Background(), "test-run", time.Second, func(ctx context.Context) {
		atomic.StoreInt32(&ran, 1)
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not run within 2s")
	}
	if atomic.LoadInt32(&ran) != 1 {
		t.Fatal("callback ran but flag not set?")
	}
}

func TestRun_RecoversFromPanic(t *testing.T) {
	// A bare `go func() { panic(...) }()` would crash the test
	// process; we're verifying that the wrapper swallows the panic.
	var wg sync.WaitGroup
	wg.Add(1)
	Run(context.Background(), "test-panic", time.Second, func(ctx context.Context) {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait() // if recover is missing, the test process dies before this returns
}

func TestRun_AppliesTimeout(t *testing.T) {
	timedOut := make(chan struct{})
	Run(context.Background(), "test-timeout", 50*time.Millisecond, func(ctx context.Context) {
		<-ctx.Done()
		close(timedOut)
	})
	select {
	case <-timedOut:
	case <-time.After(time.Second):
		t.Fatal("ctx never cancelled — timeout did not apply")
	}
}

func TestRun_ZeroTimeoutUsesDefault(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	Run(context.Background(), "test-default-timeout", 0, func(ctx context.Context) {
		dl, _ := ctx.Deadline()
		deadlineSeen <- dl
	})

	select {
	case dl := <-deadlineSeen:
		// defaultTimeout is 30s; allow generous slack for slow CI.
		untilDeadline := time.Until(dl)
		if untilDeadline < 25*time.Second || untilDeadline > 35*time.Second {
			t.Errorf("default timeout window outside [25s, 35s]: %v", untilDeadline)
		}
	case <-time.After(time.Second):
		t.Fatal("callback did not fire")
	}
}

func TestRun_DoesNotInheritParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE spawning so parent is already done

	stillRan := make(chan struct{})
	Run(parent, "test-nocancel", time.Second, func(ctx context.Context) {
		// If cancellation propagated, ctx.Err() would be non-nil
		// immediately. The contract is: parent does NOT cancel us,
		// only the timeout does.
		if ctx.Err() != nil {
			t.Errorf("background ctx already cancelled — should not inherit parent")
		}
		close(stillRan)
	})

	select {
	case <-stillRan:
	case <-time.After(time.Second):
		t.Fatal("background goroutine did not run despite cancelled parent")
	}
}
