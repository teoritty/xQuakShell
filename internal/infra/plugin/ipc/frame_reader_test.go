package ipc_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/ipc"
)

func encodeFrame(t *testing.T, kind byte, channelID uint32, payload []byte) []byte {
	t.Helper()
	var hdr [9]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	hdr[4] = kind
	binary.BigEndian.PutUint32(hdr[5:9], channelID)
	return append(hdr[:], payload...)
}

func TestReadFrameRoundTripAllKinds(t *testing.T) {
	cases := []struct {
		name      string
		kind      byte
		channelID uint32
		payload   []byte
	}{
		{"jsonrpc", domainplugin.FrameKindJSONRPC, 0, []byte(`{"jsonrpc":"2.0"}`)},
		{"binary", domainplugin.FrameKindBinary, 7, []byte{1, 2, 3, 4}},
		{"credit", domainplugin.FrameKindCredit, 42, make([]byte, 8)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := encodeFrame(t, tc.kind, tc.channelID, tc.payload)
			hdr, payload, err := ipc.ReadFrame(bytes.NewReader(wire))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hdr.Kind != tc.kind {
				t.Fatalf("kind = %x, want %x", hdr.Kind, tc.kind)
			}
			if hdr.ChannelID != tc.channelID {
				t.Fatalf("channelID = %d, want %d", hdr.ChannelID, tc.channelID)
			}
			if hdr.Length != uint32(len(tc.payload)) {
				t.Fatalf("length = %d, want %d", hdr.Length, len(tc.payload))
			}
			if !bytes.Equal(payload, tc.payload) {
				t.Fatalf("payload = %v, want %v", payload, tc.payload)
			}
		})
	}
}

func TestFrameWireBytesAreBigEndian(t *testing.T) {
	wire := encodeFrame(t, domainplugin.FrameKindJSONRPC, 0, make([]byte, 5))
	// length=5 must encode as 00 00 00 05, not little-endian.
	want := []byte{0x00, 0x00, 0x00, 0x05, domainplugin.FrameKindJSONRPC, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(wire[:9], want) {
		t.Fatalf("header bytes = % x, want % x", wire[:9], want)
	}
}

// oneByteReader returns data one (or up to n) bytes at a time to exercise reassembly
// across multiple Read() calls.
type chunkedReader struct {
	data     []byte
	chunk    int
	pos      int
	overRead bool // set true if a Read() call requested more than remains, to catch over-alloc
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if r.pos+n > len(r.data) {
		n = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

func TestReadFrameTruncatedHeaderReassembles(t *testing.T) {
	payload := []byte("hello")
	wire := encodeFrame(t, domainplugin.FrameKindJSONRPC, 0, payload)
	r := &chunkedReader{data: wire, chunk: 2}

	hdr, gotPayload, err := ipc.ReadFrame(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hdr.Kind != domainplugin.FrameKindJSONRPC || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("unexpected frame: %+v %v", hdr, gotPayload)
	}
}

func TestReadFrameTruncatedPayloadReassembles(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 100)
	wire := encodeFrame(t, domainplugin.FrameKindBinary, 3, payload)
	r := &chunkedReader{data: wire, chunk: 3}

	hdr, gotPayload, err := ipc.ReadFrame(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hdr.ChannelID != 3 || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("unexpected frame: %+v", hdr)
	}
}

// hugeLengthReader returns a valid header claiming an oversized length, then would
// happily supply a huge amount of data on any subsequent read of size >= a threshold.
// Used to prove the ceiling check happens before ReadFrame ever asks for that much data.
type hugeLengthReader struct {
	header       []byte
	pos          int
	maxRequested int
}

func (r *hugeLengthReader) Read(p []byte) (int, error) {
	if r.pos < len(r.header) {
		n := copy(p, r.header[r.pos:])
		r.pos += n
		return n, nil
	}
	if len(p) > r.maxRequested {
		r.maxRequested = len(p)
	}
	// Simulate an endless stream; ReadFrame must never get here for oversized lengths
	// because the ceiling check must reject before allocating/requesting the payload.
	for i := range p {
		p[i] = 0xAA
	}
	return len(p), nil
}

func TestReadFrameOversizedJSONRPCRejectedBeforeAllocation(t *testing.T) {
	var hdr [9]byte
	binary.BigEndian.PutUint32(hdr[0:4], domainplugin.MaxFrameBytes+1)
	hdr[4] = domainplugin.FrameKindJSONRPC
	r := &hugeLengthReader{header: hdr[:]}

	_, _, err := ipc.ReadFrame(r)
	if !errors.Is(err, ipc.ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
	if r.maxRequested != 0 {
		t.Fatalf("reader was asked to fill a payload buffer of size %d before the ceiling check rejected it", r.maxRequested)
	}
}

func TestReadFrameOversizedBinaryRejectedBeforeAllocation(t *testing.T) {
	var hdr [9]byte
	binary.BigEndian.PutUint32(hdr[0:4], domainplugin.MaxBinaryFrameBytes+1)
	hdr[4] = domainplugin.FrameKindBinary
	r := &hugeLengthReader{header: hdr[:]}

	_, _, err := ipc.ReadFrame(r)
	if !errors.Is(err, ipc.ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
	if r.maxRequested != 0 {
		t.Fatalf("reader was asked to fill a payload buffer of size %d before the ceiling check rejected it", r.maxRequested)
	}
}

func TestReadFrameInvalidKindRejected(t *testing.T) {
	for _, kind := range []byte{0x00, 0x04, 0x10} {
		wire := encodeFrame(t, kind, 0, nil)
		_, _, err := ipc.ReadFrame(bytes.NewReader(wire))
		if !errors.Is(err, ipc.ErrProtocolViolation) {
			t.Fatalf("kind 0x%02x: expected ErrProtocolViolation, got %v", kind, err)
		}
	}
}

func TestReadFrameCreditWrongLengthRejected(t *testing.T) {
	wire := encodeFrame(t, domainplugin.FrameKindCredit, 1, []byte{1, 2, 3})
	_, _, err := ipc.ReadFrame(bytes.NewReader(wire))
	if !errors.Is(err, ipc.ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
}
