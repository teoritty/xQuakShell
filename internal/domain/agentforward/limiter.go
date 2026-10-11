package agentforward

import (
	"sync"
	"time"
)

// RequestLimiter allows at most max requests in any interval of length per. It is a sliding
// window rather than a token bucket so the bound is exact: a burst at the end of one interval and
// another at the start of the next cannot add up to twice the limit.
type RequestLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	max   int
	per   time.Duration
	times []time.Time
}

// NewRequestLimiter returns a limiter reading time from now.
func NewRequestLimiter(maxRequests int, per time.Duration, now func() time.Time) *RequestLimiter {
	return &RequestLimiter{now: now, max: maxRequests, per: per}
}

// Allow records one request, or returns ErrRateLimited without recording it. A refused request is
// not counted, so a flood does not extend its own penalty past the window.
func (l *RequestLimiter) Allow() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.per)
	kept := l.times[:0]
	for _, t := range l.times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.times = kept
	if len(l.times) >= l.max {
		return ErrRateLimited
	}
	l.times = append(l.times, now)
	return nil
}
