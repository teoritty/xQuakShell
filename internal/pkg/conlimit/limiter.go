// Package conlimit provides a mutex/cond-based concurrency limiter with context cancellation.
package conlimit

import (
	"context"
	"sync"

	"ssh-client/internal/pkg/safego"
)

// Limiter bounds the number of concurrent operations that hold an acquired slot.
type Limiter struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active int
	limit  int
}

// New creates a limiter with the given slot capacity.
func New(limit int) *Limiter {
	l := &Limiter{limit: limit}
	l.cond = sync.NewCond(&l.mu)
	return l
}

// SetLimit updates the maximum number of concurrent slots.
func (l *Limiter) SetLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	l.mu.Lock()
	l.limit = limit
	l.cond.Broadcast()
	l.mu.Unlock()
}

// Limit returns the current slot capacity.
func (l *Limiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// Acquire waits until a slot is available or ctx is cancelled.
func (l *Limiter) Acquire(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	done := make(chan struct{})
	defer close(done)
	safego.GoNamed("conlimit.acquire", func() {
		select {
		case <-ctx.Done():
			l.cond.Broadcast()
		case <-done:
		}
	})
	l.mu.Lock()
	for l.active >= l.limit {
		l.cond.Wait()
		if ctx.Err() != nil {
			l.mu.Unlock()
			return ctx.Err()
		}
	}
	l.active++
	l.mu.Unlock()
	return nil
}

// Release frees one slot and wakes a waiter.
func (l *Limiter) Release() {
	l.mu.Lock()
	l.active--
	l.cond.Signal()
	l.mu.Unlock()
}
