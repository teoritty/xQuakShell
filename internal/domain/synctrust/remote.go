package synctrust

import (
	"errors"
	"fmt"
	"time"
)

// Errors a sync server can answer with, one per wire error code. The client maps the code onto
// these so the engine decides by errors.Is, and nothing the server sends beyond the bare code - no
// text, no detail - reaches the engine or the user.
var (
	ErrRemoteMalformed    = errors.New("sync server: request refused as malformed")
	ErrRemoteUnauthorized = errors.New("sync server: not signed in")
	ErrRemoteForbidden    = errors.New("sync server: not allowed")
	ErrRemoteNotFound     = errors.New("sync server: not found")
	ErrRemoteTooLarge     = errors.New("sync server: request too large")
	ErrRemoteUnsupported  = errors.New("sync server: protocol version or suite not supported")
	ErrRemoteFrozen       = errors.New("sync server: account frozen")
	ErrRemoteQuota        = errors.New("sync server: storage quota exceeded")
	ErrRemoteInternal     = errors.New("sync server: internal error")
	// ErrRemoteProtocol is a reply that breaks the contract: an unknown error code, a body that does
	// not decode, a status that means nothing here. It is treated as hostile, never retried blindly.
	ErrRemoteProtocol = errors.New("sync server: reply breaks the protocol")
	// ErrServerIdentity is a server whose TLS key is not signed by the pinned identity key, or whose
	// identity key is not the pinned one. Nothing is sent to it.
	ErrServerIdentity = errors.New("sync server: identity does not match the pinned key")
)

// ConflictError is a commit refused because the vault moved on; the client pulls and rebases onto
// CurrentSeq.
type ConflictError struct {
	CurrentSeq uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("sync server: vault moved on to seq %d", e.CurrentSeq)
}

// RateLimitedError carries how long the server asked the client to wait.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("sync server: rate limited, retry after %s", e.RetryAfter)
}
