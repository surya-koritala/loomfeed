// Package safego provides a tiny wrapper around `go func()` for
// fire-and-forget background work that should not crash the process
// or hang forever.
//
// Replace ad-hoc patterns like:
//
//	go func() {
//	    ctx := context.Background()
//	    _ = someRepo.RecordEvent(ctx, ...)
//	}()
//
// with:
//
//	safego.Run(ctx, "endorse-rep", 30*time.Second, func(ctx context.Context) {
//	    _ = someRepo.RecordEvent(ctx, ...)
//	})
//
// The wrapper:
//   - recovers from panics and logs the panic + stack at ERROR
//   - applies a default 30s timeout when the caller passes 0
//   - derives the goroutine's context from `context.Background()` so
//     the request cancellation does NOT cancel the background work
//     (the whole point of fire-and-forget) — but a separate timeout
//     still bounds runtime.
//
// This is NOT a worker pool. Each call still spawns a goroutine; the
// goal is hardening, not concurrency limiting. If a callsite needs
// strict concurrency limits (downstream rate-limited service, etc.)
// use errgroup or a semaphore instead.
package safego

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

const defaultTimeout = 30 * time.Second

// Run spawns fn in a recovered, time-bounded goroutine. `name` shows
// up in the error log if fn panics; pass something searchable like
// "endorse-rep" or "subscriber-fanout".
//
// `parent` is used only as a logging anchor — the new context is
// derived from context.Background() so the goroutine survives the
// caller returning. Pass r.Context() if you have it; nil also works.
//
// `timeout` of 0 uses defaultTimeout (30s). Pass a longer duration
// for legitimate long-running work (webhook fanout, long polls).
func Run(parent context.Context, name string, timeout time.Duration, fn func(ctx context.Context)) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("safego goroutine panic",
					"name", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		fn(ctx)
	}()
	// `parent` is intentionally unused at the moment but reserved for
	// future use (e.g., propagating trace IDs into the background ctx
	// without inheriting cancellation). Discard it explicitly so the
	// linter doesn't complain.
	_ = parent
}
