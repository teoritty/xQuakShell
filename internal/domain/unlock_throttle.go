package domain

import (
	"sync"
	"time"
)

const (
	// UnlockFreeAttempts is how many attempts happen with no wait between them. A person mistyping
	// a long passphrase deserves a few tries without being punished for it. The attempt after the
	// last free failure is the first one that waits.
	UnlockFreeAttempts = 3

	// UnlockBaseDelay is the wait once the free attempts are spent. It doubles from there, so the
	// fourth attempt costs a second, the fifth two, the sixth four.
	UnlockBaseDelay = time.Second

	// UnlockMaxDelay caps the growth. Two attempts a minute is slow enough that guessing is
	// hopeless and short enough that someone who genuinely forgot which of their passwords it was
	// is not locked out of their own data for the afternoon.
	UnlockMaxDelay = 30 * time.Second
)

// UnlockThrottle rate-limits master password attempts.
//
// The master password had no attempt limit of any kind: UnlockVault called straight through to the
// vault, so anything that could reach the Wails bridge could guess in a loop as fast as the machine
// allowed. scrypt at log2N=18 is the only thing that ever slowed that down, and it is a fixed cost
// per attempt, not a growing one - it makes each guess expensive without ever making the tenth
// thousandth guess harder than the first.
//
// The state is deliberately in memory and not persisted. Restarting the application clears it,
// which sounds like a hole and is not one: restarting requires either the user or code already
// running outside the WebView, and the attacker this defends against - a script in the UI, a plugin
// view - cannot do it. Persisting it would instead hand that attacker a way to lock the real user
// out of their own vault by failing on purpose.
//
// Delay rather than lockout, for the same reason. A vault that refuses its owner after N wrong
// guesses is a denial-of-service anyone who can reach the prompt can trigger.
// The zero value is a working throttle on the real clock, so a struct that embeds one needs no
// construction step. It holds a mutex, so use it by pointer once it is in use.
type UnlockThrottle struct {
	mu          sync.Mutex
	failures    int
	nextAllowed time.Time
	now         func() time.Time
}

// NewUnlockThrottle builds a throttle. now is injected so the backoff can be tested without
// sleeping; pass nil for time.Now.
func NewUnlockThrottle(now func() time.Time) *UnlockThrottle {
	return &UnlockThrottle{now: now}
}

// clock is the injected time source, or the real one. Reading it through a method is what lets the
// zero value work: a struct literal cannot run a constructor.
func (t *UnlockThrottle) clock() time.Time {
	if t.now == nil {
		return time.Now()
	}
	return t.now()
}

// Check reports whether an attempt may proceed, and how long to wait when it may not.
func (t *UnlockThrottle) Check() (retryAfter time.Duration, allowed bool) {
	if t == nil {
		return 0, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	remaining := t.nextAllowed.Sub(t.clock())
	if remaining > 0 {
		return remaining, false
	}
	return 0, true
}

// RecordFailure counts a rejected attempt and extends the wait before the next one.
func (t *UnlockThrottle) RecordFailure() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.failures++
	if t.failures < UnlockFreeAttempts {
		return
	}
	t.nextAllowed = t.clock().Add(unlockDelayFor(t.failures))
}

// RecordSuccess clears the history. The password was right, so nothing about the attempts before it
// says anything about the next person to sit down at this machine.
func (t *UnlockThrottle) RecordSuccess() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.failures = 0
	t.nextAllowed = time.Time{}
}

// Failures returns the consecutive failure count, for callers that log or display it.
func (t *UnlockThrottle) Failures() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failures
}

// unlockDelayFor doubles from UnlockBaseDelay once the free attempts are used up, capped at
// UnlockMaxDelay. The shift is bounded before it happens: at 63 it would overflow the duration and
// wrap to a negative wait, which is the one arithmetic mistake here that would disable the throttle
// rather than merely mistune it.
func unlockDelayFor(failures int) time.Duration {
	steps := failures - UnlockFreeAttempts
	if steps < 0 {
		return 0
	}
	if steps > 32 {
		return UnlockMaxDelay
	}
	delay := UnlockBaseDelay << uint(steps)
	if delay > UnlockMaxDelay || delay <= 0 {
		return UnlockMaxDelay
	}
	return delay
}
