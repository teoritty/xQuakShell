package main

import (
	"context"
	"log/slog"
	"time"

	"xquakshell/internal/pkg/safego"
)

// janitorInterval is how often the server prunes vaults nobody writes to. Commits prune their own
// vault at once; this pass only catches history and epochs that aged out in a quiet vault, where an
// hour of lateness costs nothing the retention window does not already allow.
const janitorInterval = time.Hour

// startJanitor runs prune every interval until ctx ends. A failed pass is logged and retried at the
// next tick: pruning is idempotent, and stopping it would let deleted data outlive its window.
func startJanitor(ctx context.Context, prune func(context.Context) error, every time.Duration, log *slog.Logger) {
	run := func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// select picks at random when the tick and the cancellation are both ready.
				if ctx.Err() != nil {
					return
				}
				if err := prune(ctx); err != nil && ctx.Err() == nil {
					log.Error("pruning failed", "err", err)
				}
			}
		}
	}
	safego.GoNamed("xqsyncd.janitor", run)
}
