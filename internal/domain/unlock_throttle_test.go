package domain

import (
	"sync"
	"testing"
	"time"
)

// fakeClock advances only when a test says so, so the backoff can be exercised without sleeping.
// A throttle test that waited for real time would be slow when it passes and flaky when it does not.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Someone mistyping a long passphrase gets a few tries. A throttle that bit on the second attempt
// would be a worse experience than the risk it removes.
func TestUnlockThrottleAllowsTheFreeAttempts(t *testing.T) {
	throttle := NewUnlockThrottle(newFakeClock().Now)

	for i := range UnlockFreeAttempts {
		if _, allowed := throttle.Check(); !allowed {
			t.Fatalf("attempt %d refused; the first %d must cost nothing", i+1, UnlockFreeAttempts)
		}
		throttle.RecordFailure()
	}
	if _, allowed := throttle.Check(); allowed {
		t.Fatalf("attempt %d allowed; the throttle must engage once the free attempts are spent", UnlockFreeAttempts+1)
	}
}

// The whole point is that guessing gets more expensive, not merely expensive: scrypt already
// charges a fixed cost per attempt and never made the ten-thousandth guess harder than the first.
func TestUnlockThrottleBackoffGrows(t *testing.T) {
	clock := newFakeClock()
	throttle := NewUnlockThrottle(clock.Now)

	for range UnlockFreeAttempts {
		throttle.RecordFailure()
	}

	var previous time.Duration
	for i := range 4 {
		throttle.RecordFailure()
		wait, allowed := throttle.Check()
		if allowed {
			t.Fatalf("failure %d imposed no wait", UnlockFreeAttempts+i+1)
		}
		if wait <= previous {
			t.Fatalf("wait after failure %d was %s, not longer than the previous %s", UnlockFreeAttempts+i+1, wait, previous)
		}
		previous = wait
		clock.Advance(wait)
	}
}

// Unbounded growth would turn a forgotten password into a lockout measured in days, and a shift
// left far enough overflows the duration and wraps negative - which would switch the throttle off
// rather than merely mistune it.
func TestUnlockThrottleBackoffIsCapped(t *testing.T) {
	clock := newFakeClock()
	throttle := NewUnlockThrottle(clock.Now)

	for i := range 200 {
		throttle.RecordFailure()
		wait, _ := throttle.Check()
		if wait > UnlockMaxDelay {
			t.Fatalf("wait after %d failures was %s, above the %s cap", i+1, wait, UnlockMaxDelay)
		}
		if wait < 0 {
			t.Fatalf("wait after %d failures was negative (%s); the backoff overflowed and the throttle is off", i+1, wait)
		}
		clock.Advance(UnlockMaxDelay)
	}
}

func TestUnlockThrottleAllowsTheAttemptOnceTheWaitElapses(t *testing.T) {
	clock := newFakeClock()
	throttle := NewUnlockThrottle(clock.Now)

	for range UnlockFreeAttempts + 1 {
		throttle.RecordFailure()
	}
	wait, allowed := throttle.Check()
	if allowed {
		t.Fatal("expected a wait after exceeding the free attempts")
	}

	clock.Advance(wait)

	if _, allowed := throttle.Check(); !allowed {
		t.Fatal("still refused after the wait elapsed; the throttle became a lockout")
	}
}

// The password was right, so nothing about the attempts before it says anything about the next
// person to sit down at this machine. Carrying the count forward would let an attacker who fails on
// purpose slow the real user down afterwards.
func TestUnlockThrottleResetsOnSuccess(t *testing.T) {
	clock := newFakeClock()
	throttle := NewUnlockThrottle(clock.Now)

	for range 10 {
		throttle.RecordFailure()
	}
	throttle.RecordSuccess()

	if _, allowed := throttle.Check(); !allowed {
		t.Fatal("a correct password left the throttle engaged")
	}
	if throttle.Failures() != 0 {
		t.Errorf("Failures() = %d after success, want 0", throttle.Failures())
	}
}

// UnlockVault can be called concurrently from the bridge, and a data race here would be a race on
// the counter that decides whether guessing is allowed.
func TestUnlockThrottleIsSafeUnderConcurrentUse(t *testing.T) {
	throttle := NewUnlockThrottle(newFakeClock().Now)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			throttle.Check()
			throttle.RecordFailure()
			throttle.Failures()
		}()
	}
	wg.Wait()

	if got := throttle.Failures(); got != 50 {
		t.Errorf("Failures() = %d after 50 concurrent failures, want 50", got)
	}
}

// A nil throttle must not panic and must not silently block every attempt: a wiring mistake should
// degrade to the old behaviour, not to an application nobody can unlock.
func TestUnlockThrottleNilIsPermissive(t *testing.T) {
	var throttle *UnlockThrottle

	if _, allowed := throttle.Check(); !allowed {
		t.Error("a nil throttle refused an attempt")
	}
	throttle.RecordFailure()
	throttle.RecordSuccess()
	if throttle.Failures() != 0 {
		t.Error("a nil throttle reported failures")
	}
}

// AppAPI embeds this by value so that wiring it costs no line in a composition root already at its
// size budget. That only works if the zero value is a working throttle.
func TestUnlockThrottleZeroValueWorks(t *testing.T) {
	var throttle UnlockThrottle

	for range UnlockFreeAttempts {
		if _, allowed := throttle.Check(); !allowed {
			t.Fatal("a zero-value throttle refused a free attempt")
		}
		throttle.RecordFailure()
	}
	if _, allowed := throttle.Check(); allowed {
		t.Fatal("a zero-value throttle never engages; it is running on no clock at all")
	}
}
