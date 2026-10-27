package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestJanitorKeepsPruningAfterAFailureAndStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := make(chan struct{}, 8)
	var passes atomic.Int64
	prune := func(context.Context) error {
		passes.Add(1)
		select {
		case calls <- struct{}{}:
		default:
		}
		return errors.New("disk busy")
	}
	startJanitor(ctx, prune, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := range 2 {
		select {
		case <-calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("pass %d never ran; a failed pass must not stop the janitor", i+1)
		}
	}
	cancel()
	// A pass that checked the context just before cancel may still enter prune with it already
	// cancelled; nothing can close that window, and the pass is harmless. What must not happen is a
	// pass after that one, so the count is taken once the in-flight pass has had time to finish and
	// must then stay put across many more ticks.
	time.Sleep(50 * time.Millisecond)
	settled := passes.Load()
	time.Sleep(50 * time.Millisecond)
	if got := passes.Load(); got != settled {
		t.Fatalf("passes went from %d to %d after the context ended; the janitor must stop with it", settled, got)
	}
}
