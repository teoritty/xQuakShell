package ipc

import (
	"io"
	"math"
	"sync"
)

// FrameWriter serializes frame writes so header+payload are emitted atomically, without
// interleaving from concurrent callers.
type FrameWriter struct {
	w  io.Writer
	mu sync.Mutex
}

// NewFrameWriter creates a FrameWriter writing frames to w.
func NewFrameWriter(w io.Writer) *FrameWriter {
	return &FrameWriter{w: w}
}

// Write encodes and writes one frame. Oversize payloads are rejected before anything is
// written to the wire.
func (fw *FrameWriter) Write(kind byte, channelID uint32, payload []byte) error {
	// Check the width before narrowing, not after: validateFrameHeader inspects the
	// already-truncated uint32, so a payload of 4 GiB+n would wrap to n and sail past
	// the ceiling check as a tiny frame.
	if uint64(len(payload)) > math.MaxUint32 {
		return newProtocolViolation("payload length %d exceeds frame header width", len(payload))
	}
	// #nosec G115 -- bounded by the explicit MaxUint32 check above.
	hdr := FrameHeader{Length: uint32(len(payload)), Kind: kind, ChannelID: channelID}
	if err := validateFrameHeader(hdr); err != nil {
		return err
	}

	encoded := encodeHeader(hdr)
	buf := make([]byte, 0, len(encoded)+len(payload))
	buf = append(buf, encoded[:]...)
	buf = append(buf, payload...)

	fw.mu.Lock()
	defer fw.mu.Unlock()
	_, err := fw.w.Write(buf)
	return err
}
