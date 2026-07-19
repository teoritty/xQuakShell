package ipc

import (
	"errors"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// TestClosedChannelTolerardsInFlightFramesDuringGrace: when the host closes a channel, the
// plugin may already have frames on the wire for it. Those must be dropped as no-ops. Removing
// the channel from the mux at close time instead would make each of them "frame for unknown
// channel id" — a protocol violation that kills a plugin for doing nothing wrong, which is the
// same defect as an empty mux, only moved.
func TestClosedChannelToleratesInFlightFramesDuringGrace(t *testing.T) {
	mux := newChannelMux(nil)
	ch := mux.Register(1, domainplugin.PurposeExec, domainplugin.DefaultChannelThroughputKbps)
	ch.Close()

	err := mux.Dispatch(FrameHeader{Length: 1, Kind: domainplugin.FrameKindBinary, ChannelID: 1}, []byte("x"))
	if err != nil {
		t.Fatalf("in-flight frame after close = %v, want silent no-op", err)
	}
}

// TestClosedChannelIsRemovedAfterGrace: the flip side. A channel that stays tracked forever is
// a leak — one map entry per channel ever opened, for the life of the plugin process.
func TestClosedChannelIsRemovedAfterGrace(t *testing.T) {
	mux := newChannelMux(nil)
	mux.closedGrace = 20 * time.Millisecond
	ch := mux.Register(1, domainplugin.PurposeExec, domainplugin.DefaultChannelThroughputKbps)

	mux.CloseAndRelease(ch.id)

	deadline := time.After(2 * time.Second)
	for {
		if _, tracked := mux.Get(1); !tracked {
			break
		}
		select {
		case <-deadline:
			t.Fatal("closed channel still tracked by the mux after the grace period; the map grows per channel opened")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// Past the grace period the plugin has no excuse: frames for a released id are violations.
	err := mux.Dispatch(FrameHeader{Length: 1, Kind: domainplugin.FrameKindBinary, ChannelID: 1}, []byte("x"))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("frame for a released channel = %v, want ErrProtocolViolation", err)
	}
}

// TestCloseAndReleaseUnblocksParkedConsumer: a backend pump parked in Recv must be released, or
// its goroutine outlives the channel forever.
func TestCloseAndReleaseUnblocksParkedConsumer(t *testing.T) {
	mux := newChannelMux(nil)
	ch := mux.Register(1, domainplugin.PurposeExec, domainplugin.DefaultChannelThroughputKbps)

	recvDone := make(chan struct{})
	go func() {
		defer close(recvDone)
		ch.Recv()
	}()

	mux.CloseAndRelease(1)

	select {
	case <-recvDone:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer parked in Recv was not released by close; its goroutine leaks")
	}
}

// TestReleaseAllRemovesEveryChannel covers connection teardown: nothing may be left tracked,
// grace timers included.
func TestReleaseAllRemovesEveryChannel(t *testing.T) {
	mux := newChannelMux(nil)
	for id := uint32(1); id <= 3; id++ {
		mux.Register(id, domainplugin.PurposeExec, domainplugin.DefaultChannelThroughputKbps)
	}

	mux.ReleaseAll()

	for id := uint32(1); id <= 3; id++ {
		if _, tracked := mux.Get(id); tracked {
			t.Fatalf("channel %d still tracked after ReleaseAll", id)
		}
	}
}
