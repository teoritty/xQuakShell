// Package syncwire is the JSON contract between the sync client and xqsyncd (ADR-023): request and
// response messages, the wire form of envelopes, heads and log entries, their conversion to and
// from vaultsync types, and the strict decoder both sides use.
//
// It is the one place the wire shape is written down, so a change to it shows up in the tests of
// both programs at once. Conversion validates identifiers and lengths but verifies no signature:
// that is vaultsync's job, done after conversion.
package syncwire

import (
	"errors"
	"slices"
)

// ErrMalformed is any message that does not decode into exactly the shape this package defines.
var ErrMalformed = errors.New("malformed sync message")

// ErrTooLarge accompanies ErrMalformed when the reason is size, so a server can answer 413.
var ErrTooLarge = errors.New("sync message exceeds its size limit")

// Error codes are the only thing an error response body ever carries. A closed list of bare
// tokens means no handler can leak an internal error string by accident: anything else is refused
// by IsErrorCode before it is written.
const (
	CodeMalformed          = "malformed"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeTooLarge           = "too_large"
	CodeRateLimited        = "rate_limited"
	CodeUnsupportedVersion = "unsupported_version"
	CodeFrozen             = "frozen"
	CodeQuotaExceeded      = "quota_exceeded"
	CodeInternal           = "internal"
)

var errorCodes = []string{
	CodeMalformed, CodeUnauthorized, CodeForbidden, CodeNotFound, CodeConflict, CodeTooLarge,
	CodeRateLimited, CodeUnsupportedVersion, CodeFrozen, CodeQuotaExceeded, CodeInternal,
}

// ErrorCodes lists every code an error response may carry.
func ErrorCodes() []string { return slices.Clone(errorCodes) }

// IsErrorCode reports whether c is one of ErrorCodes.
func IsErrorCode(c string) bool { return slices.Contains(errorCodes, c) }

// ErrorBody is the whole body of every error response.
type ErrorBody struct {
	Error string `json:"error"`
}
