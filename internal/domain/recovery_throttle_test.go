package domain

import (
	"testing"
	"time"
)

// The recovery key must be harder to grind at than the password. A password is typed from memory
// and mistyped often; a recovery key is read off paper, so a second failure already suggests
// something other than a person is at the prompt.
func TestTheRecoveryThrottleIsStricterThanThePasswordOne(t *testing.T) {
	recovery := RecoveryPolicy()
	password := DefaultUnlockPolicy()

	if recovery.FreeAttempts >= password.FreeAttempts {
		t.Errorf("recovery free attempts = %d, password = %d; the key must not be as forgiving as the password", recovery.FreeAttempts, password.FreeAttempts)
	}
	if recovery.BaseDelay <= password.BaseDelay {
		t.Errorf("recovery base delay = %s, password = %s; the first wait must be longer", recovery.BaseDelay, password.BaseDelay)
	}
	if recovery.MaxDelay <= password.MaxDelay {
		t.Errorf("recovery cap = %s, password = %s; the ceiling must be higher", recovery.MaxDelay, password.MaxDelay)
	}
}

// A throttle built with a policy has to follow that policy rather than the package defaults. The
// zero value falls back to the password's schedule, so a constructor that dropped the policy on the
// floor would still look like a working throttle.
func TestARecoveryThrottleFollowsItsOwnSchedule(t *testing.T) {
	clock := newFakeClock()
	throttle := NewRecoveryThrottle(clock.Now)
	policy := RecoveryPolicy()

	for range policy.FreeAttempts {
		if _, allowed := throttle.Check(); !allowed {
			t.Fatal("a free attempt was refused")
		}
		throttle.RecordFailure()
	}

	retryAfter, allowed := throttle.Check()
	if allowed {
		t.Fatal("the attempt after the free ones was allowed through with no wait")
	}
	if retryAfter < policy.BaseDelay {
		t.Errorf("wait = %s, want at least the recovery base delay %s; the password's one-second schedule is not strict enough here", retryAfter, policy.BaseDelay)
	}
}

// The two counters are independent objects, so exhausting one must leave the other untouched. A
// shared counter would let a morning of password typos lock the recovery prompt, and would let a
// script grinding keys hide inside the password's free attempts.
func TestTheTwoThrottlesDoNotShareState(t *testing.T) {
	clock := newFakeClock()
	password := NewUnlockThrottle(clock.Now)
	recovery := NewRecoveryThrottle(clock.Now)

	for range 10 {
		recovery.RecordFailure()
	}
	if _, allowed := recovery.Check(); allowed {
		t.Fatal("ten recovery failures left the recovery throttle open")
	}
	if _, allowed := password.Check(); !allowed {
		t.Error("recovery failures closed the password throttle; a lost key would now block the password too")
	}

	recovery.RecordSuccess()
	for range UnlockFreeAttempts + 2 {
		password.RecordFailure()
	}
	if _, allowed := password.Check(); allowed {
		t.Fatal("password failures left the password throttle open")
	}
	if _, allowed := recovery.Check(); !allowed {
		t.Error("password failures closed the recovery throttle; someone who forgot their password could not use their key")
	}
}

// The recovery backoff must keep growing and then stop at its cap, never wrap to a negative wait.
func TestTheRecoveryBackoffGrowsToItsCapAndStops(t *testing.T) {
	clock := newFakeClock()
	throttle := NewRecoveryThrottle(clock.Now)
	policy := RecoveryPolicy()

	var last time.Duration
	for i := range 40 {
		throttle.RecordFailure()
		wait, allowed := throttle.Check()
		if allowed && i >= policy.FreeAttempts {
			t.Fatalf("failure %d left the throttle open", i)
		}
		if wait < 0 {
			t.Fatalf("failure %d produced a negative wait of %s; the shift overflowed and the throttle is now disabled", i, wait)
		}
		if wait > policy.MaxDelay {
			t.Fatalf("failure %d waits %s, past the cap of %s", i, wait, policy.MaxDelay)
		}
		if wait < last {
			t.Fatalf("failure %d waits %s, less than the %s before it", i, wait, last)
		}
		last = wait
		clock.Advance(wait)
	}
	if last != policy.MaxDelay {
		t.Errorf("the backoff settled at %s, want the cap %s", last, policy.MaxDelay)
	}
}
