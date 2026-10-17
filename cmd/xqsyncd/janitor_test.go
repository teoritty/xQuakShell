package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestJanitorKeepsPruningAfterAFailureAndStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := make(chan struct{}, 8)
	stopped := make(chan struct{})
	prune := func(ctx context.Context) error {
		select {
		case calls <- struct{}{}:
		default:
		}
		if ctx.Err() != nil {
			close(stopped)
		}
		return errors.New("disk busy")
	}
	startJanitor(ctx, prune, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("pass %d never ran; a failed pass must not stop the janitor", i+1)
		}
	}
	cancel()
	select {
	case <-stopped:
		t.Fatal("the janitor pruned after its context ended")
	case <-time.After(50 * time.Millisecond):
	}
}
