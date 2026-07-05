package domain

// RateLimiter is a token-bucket byte throughput limiter.
// AllowN is non-blocking: false means the caller should reject immediately.
type RateLimiter interface {
	AllowN(n int) bool
}

// RateLimiterFactory mints per-session limiters (embed creates one per token).
type RateLimiterFactory interface {
	New(bytesPerSec int, burst int) RateLimiter
}
