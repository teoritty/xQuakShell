package ipc

import (
	"encoding/binary"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// creditPayload builds a well-formed kind=0x03 payload per ADR-011: 4B channelId + 4B credit.
func creditPayload(channelID, credit uint32) []byte {
	p := make([]byte, 8)
	binary.BigEndian.PutUint32(p[0:4], channelID)
	binary.BigEndian.PutUint32(p[4:8], credit)
	return p
}

// registerFlow puts a flow-controlled channel into the mux directly. mux.Register builds a
// bare newChannel (no credit window), which is exactly the production gap under repair; these
// tests need the flow-control wiring the composition root will use.
func registerFlow(m *channelMux, id uint32, purpose string) *channel {
	ch := newFlowChannel(id, purpose, domainplugin.InitialCredit(purpose), domainplugin.DefaultChannelThroughputKbps, nil, func([]byte) error { return nil })
	m.mu.Lock()
	m.channels[id] = ch
	m.mu.Unlock()
	return ch
}

func creditHeader(channelID uint32) FrameHeader {
	return FrameHeader{Length: 8, Kind: domainplugin.FrameKindCredit, ChannelID: channelID}
}

// TestCreditFrameReplenishesOutboundWindow is the F-3 regression: a kind=0x03 frame must
// replenish the channel's outbound credit, not land in the consumer's data queue.
func TestCreditFrameReplenishesOutboundWindow(t *testing.T) {
	mux := newChannelMux(nil)
	ch := registerFlow(mux, 1, domainplugin.PurposeExec)

	initial := domainplugin.InitialCredit(domainplugin.PurposeExec)
	// Drain the whole outbound window so replenishment is observable.
	for i := 0; i < initial; i++ {
		if !ch.credit.TryAcquireOutbound() {
			t.Fatalf("expected %d initial outbound credit, exhausted after %d", initial, i)
		}
	}
	if got := ch.credit.AvailableOutbound(); got != 0 {
		t.Fatalf("outbound credit = %d after draining, want 0", got)
	}

	if err := mux.Dispatch(creditHeader(1), creditPayload(1, 3)); err != nil {
		t.Fatalf("dispatch credit frame: %v", err)
	}

	if got := ch.credit.AvailableOutbound(); got != 3 {
		t.Fatalf("outbound credit = %d after 0x03 grant of 3, want 3", got)
	}
}

// TestCreditFrameIsNotDeliveredAsData guards the second half of F-3: a credit frame handed to
// the consumer as an 8-byte payload silently corrupts the data stream.
func TestCreditFrameIsNotDeliveredAsData(t *testing.T) {
	mux := newChannelMux(nil)
	ch := registerFlow(mux, 1, domainplugin.PurposeExec)

	if err := mux.Dispatch(creditHeader(1), creditPayload(1, 1)); err != nil {
		t.Fatalf("dispatch credit frame: %v", err)
	}

	if f, ok := drainNonBlocking(ch); ok {
		t.Fatalf("credit frame leaked into the data queue as payload %q", f.Payload)
	}
}

// TestCreditFrameForeignChannelIDIsProtocolViolation: the payload's channelId must match the
// frame header's. A mismatch would let a plugin grant credit to a channel it does not own.
func TestCreditFrameForeignChannelIDIsProtocolViolation(t *testing.T) {
	mux := newChannelMux(nil)
	registerFlow(mux, 1, domainplugin.PurposeExec)
	registerFlow(mux, 2, domainplugin.PurposeExec)

	err := mux.Dispatch(creditHeader(1), creditPayload(2, 4))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for cross-channel credit grant, got %v", err)
	}
}

func TestCreditFrameZeroGrantIsProtocolViolation(t *testing.T) {
	mux := newChannelMux(nil)
	registerFlow(mux, 1, domainplugin.PurposeExec)

	err := mux.Dispatch(creditHeader(1), creditPayload(1, 0))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for a zero-credit grant, got %v", err)
	}
}

// TestCreditFrameOverflowIsProtocolViolation: ReplenishOutbound sums without a ceiling, so an
// unbounded grant loop would overflow the window rather than be refused.
func TestCreditFrameOverflowIsProtocolViolation(t *testing.T) {
	mux := newChannelMux(nil)
	registerFlow(mux, 1, domainplugin.PurposeExec)

	err := mux.Dispatch(creditHeader(1), creditPayload(1, ^uint32(0)))
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for a credit grant past the window ceiling, got %v", err)
	}
}
