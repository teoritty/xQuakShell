package ipc

import (
	"sync"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// newTestChannel builds a channel the way production does — flow-controlled — with a discarding
// writer, since these tests are about lifecycle and delivery rather than the wire.
func newTestChannel(id uint32) *channel {
	return newFlowChannel(id, domainplugin.PurposeExec, domainplugin.InitialCredit(domainplugin.PurposeExec),
		domainplugin.DefaultChannelThroughputKbps, nil, func([]byte) error { return nil })
}

func TestChannelCloseIsIdempotentUnderConcurrentCallers(t *testing.T) {
	ch := newTestChannel(1)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Close()
		}()
	}
	wg.Wait()

	if !ch.Closed() {
		t.Fatal("expected channel to be closed")
	}

	// Calling Close() again afterwards must still be safe (no panic, no double-close side
	// effect: the closed channel must not be closed twice).
	ch.Close()
}

func TestChannelDropsInboundFramesAfterLocalCloseAsNoOp(t *testing.T) {
	ch := newTestChannel(1)
	ch.deliver(domainplugin.FrameKindBinary, []byte("before close"))

	ch.Close()

	ch.deliver(domainplugin.FrameKindBinary, []byte("after close"))

	f, ok := ch.Recv()
	if !ok {
		t.Fatal("expected the frame queued before close to still be delivered")
	}
	if string(f.Payload) != "before close" {
		t.Fatalf("unexpected payload %q", f.Payload)
	}

	// Nothing else should ever arrive: the post-close deliver was a silent no-op, not an
	// error and not queued.
	_, ok = ch.Recv()
	if ok {
		t.Fatal("expected no further frames after close, got one")
	}
}

func TestChannelRecvBlocksUntilDelivered(t *testing.T) {
	ch := newTestChannel(1)

	done := make(chan channelFrame, 1)
	go func() {
		f, ok := ch.Recv()
		if ok {
			done <- f
		}
	}()

	select {
	case <-done:
		t.Fatal("Recv returned before any frame was delivered")
	case <-time.After(50 * time.Millisecond):
	}

	ch.deliver(domainplugin.FrameKindBinary, []byte("payload"))

	select {
	case f := <-done:
		if string(f.Payload) != "payload" {
			t.Fatalf("unexpected payload %q", f.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Recv did not unblock after delivery")
	}
}
