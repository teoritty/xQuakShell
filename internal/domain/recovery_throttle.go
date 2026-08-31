package domain

import "time"

const (
	// RecoveryFreeAttempts is deliberately one, against the master password's three. Nobody types a
	// recovery key from memory: it is read off paper or pasted, so a second attempt is already a
	// sign that something other than a person is at the prompt.
	RecoveryFreeAttempts = 1

	// RecoveryBaseDelay starts five times higher than the password's second, and
	// RecoveryMaxDelay caps ten times higher. The key is 160 bits, so guessing it was never the
	// realistic attack - what this actually buys is that a script in the WebView cannot grind the
	// prompt looking for an implementation mistake without the user noticing the application has
	// gone unresponsive for minutes.
	RecoveryBaseDelay = 5 * time.Second
	RecoveryMaxDelay  = 5 * time.Minute
)

// ThrottlePolicy is the backoff schedule a throttle follows: how many attempts pass freely, what
// the first wait after those costs, and how far the doubling is allowed to run.
type ThrottlePolicy struct {
	FreeAttempts int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
}

// DefaultUnlockPolicy is the master password's schedule, and the one a zero-value UnlockThrottle
// follows.
func DefaultUnlockPolicy() ThrottlePolicy {
	return ThrottlePolicy{
		FreeAttempts: UnlockFreeAttempts,
		BaseDelay:    UnlockBaseDelay,
		MaxDelay:     UnlockMaxDelay,
	}
}

// RecoveryPolicy is the recovery key's schedule.
func RecoveryPolicy() ThrottlePolicy {
	return ThrottlePolicy{
		FreeAttempts: RecoveryFreeAttempts,
		BaseDelay:    RecoveryBaseDelay,
		MaxDelay:     RecoveryMaxDelay,
	}
}

// NewRecoveryThrottle builds the counter that rate-limits recovery key attempts.
//
// It is a second, independent counter rather than a share of the password's. Two credentials open
// the vault and only one of them is memorised, so a household that mistypes its password all
// morning must not arrive at the recovery prompt already locked out - and, in the other direction,
// a script grinding recovery keys must not be able to hide inside the password's three free tries.
//
// Which counter an attempt lands on is decided by LooksLikeRecoveryKey, from the shape of the input
// alone. That tells an attacker nothing: it is a fact about the characters they just typed.
func NewRecoveryThrottle(now func() time.Time) *UnlockThrottle {
	return NewThrottleWithPolicy(now, RecoveryPolicy())
}
