package usecase

import (
	"errors"
	"fmt"
)

// EmbedRefusalCause names why the embed surface refused a plugin frame.
//
// It exists because the two sentinels the sink returns are each overloaded across meanings a
// caller must treat oppositely: ErrRateLimited means both "the frame is too large, retrying never
// helps" and "the token bucket is empty, retrying helps in milliseconds"; ErrTerminalBackpressure
// means both "the tab is inactive, this lasts minutes and is the user's own doing" and "the
// WebSocket send buffer is momentarily full". A caller cannot tell "wait" from "tear down" apart
// by sentinel, and reverse-engineering it from the sentinel — or reaching into
// EmbedTunnelService's internals to re-derive it — is not the caller's job. The service is the
// only thing that knows why it refused, so the classification is made exactly here, once.
type EmbedRefusalCause string

const (
	// EmbedRefusedFrameTooLarge is a deterministic protocol violation: the frame exceeds
	// domain.MaxTunnelFrameSize. Retrying the same frame never succeeds.
	EmbedRefusedFrameTooLarge EmbedRefusalCause = "frame-too-large"
	// EmbedRefusedRateLimited is transient: the session's throughput token bucket denied the
	// frame. Retrying succeeds once the bucket refills.
	EmbedRefusedRateLimited EmbedRefusalCause = "rate-limited"
	// EmbedRefusedTabInactive is transient and user-driven: the session's tab is backgrounded.
	// It lasts as long as the user looks elsewhere — minutes, legitimately.
	EmbedRefusedTabInactive EmbedRefusalCause = "tab-inactive"
	// EmbedRefusedWSBufferFull is transient and momentary: the browser's WebSocket send queue is
	// full because the consumer is behind. It clears in milliseconds on a healthy browser.
	EmbedRefusedWSBufferFull EmbedRefusalCause = "ws-buffer-full"
	// EmbedRefusedSessionGone is terminal: the session's embed registration was revoked or never
	// existed. Nothing about it improves with time.
	EmbedRefusedSessionGone EmbedRefusalCause = "session-revoked"
)

// EmbedFrameRefusedError reports a classified refusal of one embed tunnel frame.
//
// It WRAPS the existing sentinel rather than replacing it, and that is load-bearing rather than
// politeness: ipc.HostServer maps ErrRateLimited / ErrTerminalBackpressure / ErrSessionNotFound to
// JSON-RPC error codes with errors.Is, and RouteTunnelFrameFromPlugin is still the legacy ADR-008
// tunnel path's sink as well as the channel bus's. Every existing caller keeps matching the
// sentinel it always matched; a caller that needs the reason uses errors.As on this type.
type EmbedFrameRefusedError struct {
	Cause EmbedRefusalCause
	// sentinel is the pre-existing error this refusal continues to satisfy errors.Is against.
	sentinel error
}

func (e *EmbedFrameRefusedError) Error() string {
	return fmt.Sprintf("embed tunnel frame refused (%s): %v", e.Cause, e.sentinel)
}

// Unwrap keeps errors.Is(err, domainplugin.ErrRateLimited) and friends true.
func (e *EmbedFrameRefusedError) Unwrap() error { return e.sentinel }

// newEmbedRefusal builds a classified refusal over the sentinel the caller already returned.
func newEmbedRefusal(cause EmbedRefusalCause, sentinel error) error {
	return &EmbedFrameRefusedError{Cause: cause, sentinel: sentinel}
}

// EmbedRefusalCauseOf reports the classified cause carried by err, if any. It is the one way a
// caller is meant to learn why a frame was refused.
func EmbedRefusalCauseOf(err error) (EmbedRefusalCause, bool) {
	var refused *EmbedFrameRefusedError
	if errors.As(err, &refused) {
		return refused.Cause, true
	}
	return "", false
}
