package ipc

import (
	"errors"
	"fmt"
	"io"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ErrProtocolViolation is the sentinel for any framing anomaly. Per ADR-011 §2a this is
// fatal to the whole connection, never recoverable per-frame.
var ErrProtocolViolation = errors.New("plugin ipc: protocol violation")

// ProtocolViolationError carries the specific reason for a protocol violation.
type ProtocolViolationError struct {
	Reason string
}

func (e *ProtocolViolationError) Error() string {
	return fmt.Sprintf("plugin ipc: protocol violation: %s", e.Reason)
}

func (e *ProtocolViolationError) Unwrap() error {
	return ErrProtocolViolation
}

func newProtocolViolation(format string, args ...any) error {
	return &ProtocolViolationError{Reason: fmt.Sprintf(format, args...)}
}

// frameCeiling returns the max payload length permitted for kind, or ok=false if kind is invalid.
func frameCeiling(kind byte) (uint32, bool) {
	switch kind {
	case domainplugin.FrameKindJSONRPC:
		return domainplugin.MaxFrameBytes, true
	case domainplugin.FrameKindBinary:
		return domainplugin.MaxBinaryFrameBytes, true
	case domainplugin.FrameKindCredit:
		return 8, true
	default:
		return 0, false
	}
}

func validateFrameHeader(hdr FrameHeader) error {
	ceiling, ok := frameCeiling(hdr.Kind)
	if !ok {
		return newProtocolViolation("invalid frame kind 0x%02x", hdr.Kind)
	}
	if hdr.Kind == domainplugin.FrameKindCredit && hdr.Length != 8 {
		return newProtocolViolation("credit frame length must be exactly 8, got %d", hdr.Length)
	}
	if hdr.Length > ceiling {
		return newProtocolViolation("frame length %d exceeds ceiling %d for kind 0x%02x", hdr.Length, ceiling, hdr.Kind)
	}
	return nil
}

// ReadFrame reads and validates one frame from r. The ceiling check happens before any
// payload buffer is allocated, so a bogus length field cannot force a large allocation.
func ReadFrame(r io.Reader) (FrameHeader, []byte, error) {
	var hdrBuf [domainplugin.FrameHeaderLen]byte
	if _, err := io.ReadFull(r, hdrBuf[:]); err != nil {
		return FrameHeader{}, nil, err
	}
	hdr := decodeHeader(hdrBuf[:])

	if err := validateFrameHeader(hdr); err != nil {
		return FrameHeader{}, nil, err
	}

	payload := make([]byte, hdr.Length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return FrameHeader{}, nil, err
	}
	return hdr, payload, nil
}
