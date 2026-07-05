package domain

import "context"

// ConcurrencyLimiter bounds parallel operations that hold an acquired slot.
// Acquire blocks until a slot is free or ctx is cancelled.
// Release must be called once per successful Acquire (typically via defer).
type ConcurrencyLimiter interface {
	Acquire(ctx context.Context) error
	Release()
	SetLimit(limit int)
}
