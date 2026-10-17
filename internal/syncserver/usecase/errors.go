package usecase

import (
	"errors"
	"fmt"
)

// The use-case errors map one to one onto the transport's error codes. Anything else reaching the
// transport is an internal error and is reported as such, never by its text.
var (
	ErrMalformed     = errors.New("malformed request")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrNotFound      = errors.New("not found")
	ErrFrozen        = errors.New("account frozen")
	ErrQuotaExceeded = errors.New("quota exceeded")
	ErrTooLarge      = errors.New("request too large")
	ErrRateLimited   = errors.New("rate limited")
	ErrConflict      = errors.New("conflict")
)

// ConflictError carries the seq a stale commit must rebase onto.
type ConflictError struct {
	CurrentSeq uint64
}

func (e *ConflictError) Error() string { return fmt.Sprintf("conflict: current seq %d", e.CurrentSeq) }

// Is lets callers match the error with errors.Is(err, ErrConflict).
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// RateLimitedError carries how long the caller must wait.
type RateLimitedError struct {
	RetryAfterMillis uint64
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limited: retry after %d ms", e.RetryAfterMillis)
}

// Is lets callers match the error with errors.Is(err, ErrRateLimited).
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// malformed wraps a validation failure, keeping the cause for the server log only.
func malformed(cause error) error { return fmt.Errorf("%w: %w", ErrMalformed, cause) }
