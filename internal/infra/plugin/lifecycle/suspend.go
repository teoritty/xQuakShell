package lifecycle

import (
	"context"
	"time"
)

// IdleTarget suspends plugins that exceeded an idle threshold.
type IdleTarget interface {
	SuspendIdlePlugins(ctx context.Context, idleAfter time.Duration)
}

// Config configures the idle suspender loop.
type Config struct {
	IdleAfter time.Duration
	TickEvery time.Duration
}

// RunIdleSuspender periodically hard-suspends idle plugin processes.
func RunIdleSuspender(ctx context.Context, target IdleTarget, cfg Config) {
	if target == nil {
		return
	}
	idleAfter := cfg.IdleAfter
	if idleAfter <= 0 {
		idleAfter = 5 * time.Minute
	}
	tick := cfg.TickEvery
	if tick <= 0 {
		tick = time.Minute
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			target.SuspendIdlePlugins(ctx, idleAfter)
		}
	}
}
