package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"xquakshell/internal/domain/synctrust"
)

func TestAMoveWaitsOutTheServersRateLimit(t *testing.T) {
	var waited []time.Duration
	sleep := func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }
	calls := 0
	err := waitOutRateLimit(context.Background(), sleep, func() error {
		calls++
		if calls < 3 {
			return &synctrust.RateLimitedError{RetryAfter: 2 * time.Second}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err = %v after %d calls; a rate limit is waited out, not shown as a failed move", err, calls)
	}
	if len(waited) != 2 || waited[0] != 2*time.Second {
		t.Fatalf("waited %v; each retry waits as long as the server asked", waited)
	}
}

func TestAMoveGivesUpOnARateLimitThatDoesNotLift(t *testing.T) {
	var waited []time.Duration
	sleep := func(_ context.Context, d time.Duration) error { waited = append(waited, d); return nil }
	calls := 0
	err := waitOutRateLimit(context.Background(), sleep, func() error {
		calls++
		return &synctrust.RateLimitedError{RetryAfter: time.Hour}
	})
	var limited *synctrust.RateLimitedError
	if !errors.As(err, &limited) || calls != moveRateLimitRetries+1 {
		t.Fatalf("err = %v after %d calls; the last refusal is reported once the retries are spent", err, calls)
	}
	for _, d := range waited {
		if d > moveRateLimitMaxWait {
			t.Fatalf("waited %v; a click is never held for longer than %v at a time", d, moveRateLimitMaxWait)
		}
	}
}

func TestAMoveDoesNotRetryOtherRefusals(t *testing.T) {
	calls := 0
	boom := errors.New("not a member")
	err := waitOutRateLimit(context.Background(), func(context.Context, time.Duration) error { return nil }, func() error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err = %v after %d calls; only a rate limit is worth waiting for", err, calls)
	}
}

func TestAMoveStopsWaitingWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := waitOutRateLimit(ctx, sleepCtx, func() error {
		calls++
		return &synctrust.RateLimitedError{RetryAfter: time.Second}
	})
	if calls != 1 || err == nil {
		t.Fatalf("err = %v after %d calls; a cancelled move does not keep waiting", err, calls)
	}
}
