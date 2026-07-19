package ipc

import (
	"errors"
	"testing"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// TestEmbedStreamFrameCapMatchesTunnelLimit pins the two halves of the same rule together: the
// channel layer's per-purpose cap is the embed leg's own frame limit, restated where domain/plugin
// can see it. If either moves, the plugin's channel and the embed sink disagree again.
func TestEmbedStreamFrameCapMatchesTunnelLimit(t *testing.T) {
	if domainplugin.MaxEmbedStreamFrameBytes != domain.MaxTunnelFrameSize {
		t.Fatalf("embed-stream channel cap %d != embed tunnel frame limit %d",
			domainplugin.MaxEmbedStreamFrameBytes, domain.MaxTunnelFrameSize)
	}
}

// TestEmbedStreamOversizeFrameIsRefusedAtIngress proves D2's cap is enforced where the cause is:
// an oversize embed-stream frame is a deterministic protocol error, not backpressure, and it never
// reaches the channel's consumer -- so the embed sink never sees it and no channel is torn down by
// a sink error whose sentinel says "slow down".
func TestEmbedStreamOversizeFrameIsRefusedAtIngress(t *testing.T) {
	mux := newChannelMux(func(uint32, []byte) error { return nil })
	ch := mux.Register(7, domainplugin.PurposeEmbedStream, domainplugin.DefaultChannelThroughputKbps)

	oversize := make([]byte, domainplugin.MaxEmbedStreamFrameBytes+1)
	err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 7,
		Length: uint32(len(oversize))}, oversize)
	if err == nil {
		t.Fatal("expected an oversize embed-stream frame to be refused at ingress")
	}
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected a deterministic protocol violation, got %v", err)
	}
	if errors.Is(err, domainplugin.ErrRateLimited) || errors.Is(err, domainplugin.ErrTerminalBackpressure) {
		t.Fatalf("an oversize frame must not read as backpressure: %v", err)
	}
	if ch.Closed() {
		t.Fatal("the refusal must not close the channel")
	}
	ch.mu.Lock()
	queued := len(ch.queue)
	ch.mu.Unlock()
	if queued != 0 {
		t.Fatalf("the oversize frame reached the consumer queue (%d frames): it must never reach the sink", queued)
	}
}

// TestEmbedStreamMaxSizeFramesFlowAcrossWindows drives InitialCredit x 3 frames at exactly the cap
// (readiness 7.2: 8 frames prove nothing about a window that never has to reopen), so the ingress
// check cannot be an off-by-one that quietly refuses legal traffic.
func TestEmbedStreamMaxSizeFramesFlowAcrossWindows(t *testing.T) {
	mux := newChannelMux(func(uint32, []byte) error { return nil })
	ch := mux.Register(9, domainplugin.PurposeEmbedStream, domainplugin.DefaultChannelThroughputKbps)

	atCap := make([]byte, domainplugin.MaxEmbedStreamFrameBytes)
	total := domainplugin.InitialCredit(domainplugin.PurposeEmbedStream) * 3
	for i := 0; i < total; i++ {
		if err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 9,
			Length: uint32(len(atCap))}, atCap); err != nil {
			t.Fatalf("frame %d at exactly the cap was refused: %v", i, err)
		}
		if _, ok := ch.Recv(); !ok {
			t.Fatalf("frame %d was not delivered to the consumer", i)
		}
		// What channelDataPath.Ack does locally: the consumer took it, so the window reopens.
		ch.grantInbound(1)
	}
}

// TestOtherPurposesKeepTheOneMiBCeiling pins that the 64 KiB rule is embed-stream's alone: a
// tcp-relay frame above it is ordinary, legal traffic.
func TestOtherPurposesKeepTheOneMiBCeiling(t *testing.T) {
	mux := newChannelMux(func(uint32, []byte) error { return nil })
	mux.Register(11, domainplugin.PurposeTCPRelay, domainplugin.DefaultChannelThroughputKbps)

	payload := make([]byte, domainplugin.MaxEmbedStreamFrameBytes+1)
	if err := mux.Dispatch(FrameHeader{Kind: domainplugin.FrameKindBinary, ChannelID: 11,
		Length: uint32(len(payload))}, payload); err != nil {
		t.Fatalf("tcp-relay frame of %d bytes must stay legal: %v", len(payload), err)
	}
}
