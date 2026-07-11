package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestChannelMuxDemuxesInterleavedFramesPerChannel(t *testing.T) {
	mux := newChannelMux()
	const n = 3
	channels := make([]*channel, n)
	for i := 0; i < n; i++ {
		channels[i] = mux.Register(uint32(i + 1))
	}

	// Interleave 0x02 frames across the N channels in mixed, non-round-robin order.
	order := []int{0, 1, 2, 1, 0, 0, 2, 1, 2, 0}
	counters := make([]int, n)
	for _, idx := range order {
		payload := []byte(fmt.Sprintf("ch%d-frame%d", idx, counters[idx]))
		counters[idx]++
		if err := mux.Dispatch(FrameHeader{Length: uint32(len(payload)), Kind: domainplugin.FrameKindBinary, ChannelID: uint32(idx + 1)}, payload); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}

	for i := 0; i < n; i++ {
		want := counters[i]
		got := 0
		for {
			f, ok := drainNonBlocking(channels[i])
			if !ok {
				break
			}
			expected := fmt.Sprintf("ch%d-frame%d", i, got)
			if string(f.Payload) != expected {
				t.Fatalf("channel %d: frame %d payload = %q, want %q (cross-channel bleed)", i, got, f.Payload, expected)
			}
			got++
		}
		if got != want {
			t.Fatalf("channel %d: got %d frames, want %d", i, got, want)
		}
	}
}

// drainNonBlocking pulls one frame off ch's queue if one is already present, without
// blocking, by racing Recv (which blocks) against a short timeout.
func drainNonBlocking(ch *channel) (channelFrame, bool) {
	type result struct {
		f  channelFrame
		ok bool
	}
	out := make(chan result, 1)
	go func() {
		f, ok := ch.Recv()
		out <- result{f, ok}
	}()
	select {
	case r := <-out:
		return r.f, r.ok
	case <-time.After(100 * time.Millisecond):
		ch.Close() // unblock the leaked goroutine's Recv
		return channelFrame{}, false
	}
}

func TestChannelMuxDispatchUnknownChannelIsProtocolViolation(t *testing.T) {
	mux := newChannelMux()
	err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 99}, []byte("x"))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for unknown channel, got %v", err)
	}
}

func TestChannelMuxDispatchRemovedChannelIsProtocolViolation(t *testing.T) {
	mux := newChannelMux()
	ch := mux.Register(5)
	ch.Close()
	mux.Remove(5)

	err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 5}, []byte("x"))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for removed channel, got %v", err)
	}
}

func TestChannelMuxDispatchLocallyClosedButTrackedChannelIsSilentNoOp(t *testing.T) {
	mux := newChannelMux()
	ch := mux.Register(5)
	ch.Close()

	err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 5}, []byte("x"))
	if err != nil {
		t.Fatalf("expected no error for a closed-but-still-tracked channel, got %v", err)
	}
	if _, ok := ch.Recv(); ok {
		t.Fatal("expected no delivery to a closed channel")
	}
}

// TestChannelMuxHeadOfLineBlocking proves that channel A, whose consumer never drains it,
// does not block channel B's delivery nor the JSON-RPC (kind=0x01) control plane sharing
// the same read loop.
func TestChannelMuxHeadOfLineBlocking(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	hostInR, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInR.Close()
	})

	handler := func(ctx context.Context, method string, _ json.RawMessage) (json.RawMessage, *RPCError) {
		raw, _ := json.Marshal(map[string]bool{"ok": true})
		return raw, nil
	}

	conn := NewConn(pluginOutR, hostInW, nil, handler)
	t.Cleanup(conn.Close)

	const chanA, chanB = uint32(1), uint32(2)
	a := conn.mux.Register(chanA)
	b := conn.mux.Register(chanB)
	// Channel A is deliberately never drained (no goroutine calls a.Recv()).
	_ = a

	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		for i := 0; i < 5; i++ {
			if _, ok := b.Recv(); !ok {
				return
			}
		}
	}()

	pluginFW := NewFrameWriter(pluginOutW)

	// Flood channel A's backlog, interleaved with channel B and a JSON-RPC request that
	// the fake "plugin" on the other end (hostInR/pluginOutW) must be able to answer
	// promptly even while A is backed up.
	go func() {
		for i := 0; i < 200; i++ {
			_ = pluginFW.Write(domainplugin.FrameKindBinary, chanA, bytes.Repeat([]byte{'a'}, 64))
		}
	}()
	go func() {
		for i := 0; i < 5; i++ {
			_ = pluginFW.Write(domainplugin.FrameKindBinary, chanB, []byte("b-frame"))
		}
	}()

	// Fake plugin: read whatever the host sends on channel 0 (JSON-RPC) and answer it.
	go func() {
		r := hostInR
		for {
			hdr, payload, err := ReadFrame(r)
			if err != nil {
				return
			}
			if hdr.Kind != domainplugin.FrameKindJSONRPC {
				continue
			}
			var msg Message
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}
			if msg.ID == nil {
				continue
			}
			resp, _ := json.Marshal(map[string]bool{"ok": true})
			respMsg := NewResponse(*msg.ID, resp)
			data, _ := json.Marshal(respMsg)
			_ = pluginFW.Write(domainplugin.FrameKindJSONRPC, 0, data)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	callDone := make(chan error, 1)
	go func() {
		_, err := conn.Call(ctx, "ping", nil)
		callDone <- err
	}()

	select {
	case err := <-callDone:
		if err != nil {
			t.Fatalf("JSON-RPC ping did not complete promptly: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("JSON-RPC ping was head-of-line-blocked by channel A's backlog")
	}

	select {
	case <-bDone:
	case <-time.After(3 * time.Second):
		t.Fatal("channel B's frames were head-of-line-blocked by channel A's backlog")
	}
}
