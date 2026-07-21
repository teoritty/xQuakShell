package ipc_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/ipc"
)

func TestFrameWriterRoundTripAllKinds(t *testing.T) {
	cases := []struct {
		name      string
		kind      byte
		channelID uint32
		payload   []byte
	}{
		{"jsonrpc", domainplugin.FrameKindJSONRPC, 0, []byte(`{"jsonrpc":"2.0"}`)},
		{"binary", domainplugin.FrameKindBinary, 9, []byte{9, 8, 7}},
		{"credit", domainplugin.FrameKindCredit, 11, make([]byte, 8)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			fw := ipc.NewFrameWriter(&buf)
			if err := fw.Write(tc.kind, tc.channelID, tc.payload); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hdr, payload, err := ipc.ReadFrame(&buf)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if hdr.Kind != tc.kind || hdr.ChannelID != tc.channelID {
				t.Fatalf("unexpected header: %+v", hdr)
			}
			if !bytes.Equal(payload, tc.payload) {
				t.Fatalf("payload = %v, want %v", payload, tc.payload)
			}
		})
	}
}

func TestFrameWriterBigEndianOnWire(t *testing.T) {
	var buf bytes.Buffer
	fw := ipc.NewFrameWriter(&buf)
	if err := fw.Write(domainplugin.FrameKindJSONRPC, 0x01020304, make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	wire := buf.Bytes()
	if got := binary.BigEndian.Uint32(wire[0:4]); got != 5 {
		t.Fatalf("length bytes decode as %d, want 5 (big-endian)", got)
	}
	if wire[4] != domainplugin.FrameKindJSONRPC {
		t.Fatalf("kind byte = %x", wire[4])
	}
	if got := binary.BigEndian.Uint32(wire[5:9]); got != 0x01020304 {
		t.Fatalf("channelId bytes decode as %x, want 0x01020304 (big-endian)", got)
	}
}

func TestFrameWriterRejectsOversizedBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	fw := ipc.NewFrameWriter(&buf)
	oversized := make([]byte, domainplugin.MaxBinaryFrameBytes+1)

	err := fw.Write(domainplugin.FrameKindBinary, 1, oversized)
	if !errors.Is(err, ipc.ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing written to the wire, got %d bytes", buf.Len())
	}
}

func TestFrameWriterInvalidKindRejected(t *testing.T) {
	var buf bytes.Buffer
	fw := ipc.NewFrameWriter(&buf)
	if err := fw.Write(0x04, 0, nil); !errors.Is(err, ipc.ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing written to the wire, got %d bytes", buf.Len())
	}
}

func TestFrameWriterSerializesConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	fw := ipc.NewFrameWriter(&buf)
	payload := make([]byte, 100)

	var wg sync.WaitGroup
	const n = 50
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			_ = fw.Write(domainplugin.FrameKindBinary, id, payload)
		}(uint32(i))
	}
	wg.Wait()

	count := 0
	r := bytes.NewReader(buf.Bytes())
	for {
		_, gotPayload, err := ipc.ReadFrame(r)
		if err != nil {
			break
		}
		if len(gotPayload) != len(payload) {
			t.Fatalf("frame %d: interleaved/corrupted payload, got len %d", count, len(gotPayload))
		}
		count++
	}
	if count != n {
		t.Fatalf("read back %d frames, want %d (writes interleaved/corrupted)", count, n)
	}
}
