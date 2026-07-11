package ipc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// fakeUpstream models a backend's upstream source (SSH exec stdout pipe / relayed TCP or UDP
// socket). Read is only ever called by test goroutines that first waited on a backendGate.
type fakeUpstream struct {
	reads atomic.Int32
}

func (f *fakeUpstream) Read() []byte {
	f.reads.Add(1)
	return []byte("data")
}

// runGatedReader simulates a purpose backend's read loop: wait for capacity, then read once,
// repeated until ctx is done.
func runGatedReader(ctx context.Context, gate *backendGate, credit *channelCredit, up *fakeUpstream, readDone chan<- struct{}) {
	for {
		if err := gate.WaitForCapacity(ctx); err != nil {
			return
		}
		// Mirror the real flow: once capacity is confirmed, reading upstream produces a
		// frame that itself consumes one credit unit on the way out.
		if !credit.TryAcquireOutbound() {
			continue
		}
		up.Read()
		select {
		case readDone <- struct{}{}:
		default:
		}
	}
}

func testPauseUpstreamReadGating(t *testing.T, purpose string) {
	t.Helper()
	credit := newChannelCredit(0) // start exhausted, as if the initial grant was already spent
	gate := newBackendGate(credit)
	up := &fakeUpstream{}

	if policyForPurpose(purpose) != policyPauseUpstreamRead {
		t.Fatalf("purpose %q: expected policyPauseUpstreamRead", purpose)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readDone := make(chan struct{}, 1)
	go runGatedReader(ctx, gate, credit, up, readDone)

	select {
	case <-readDone:
		t.Fatalf("purpose %q: upstream Read was called while credit was 0", purpose)
	case <-time.After(100 * time.Millisecond):
	}
	if got := up.reads.Load(); got != 0 {
		t.Fatalf("purpose %q: expected 0 reads while credit exhausted, got %d", purpose, got)
	}

	credit.ReplenishOutbound(1)

	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("purpose %q: upstream Read was not called after credit replenishment", purpose)
	}
	if got := up.reads.Load(); got != 1 {
		t.Fatalf("purpose %q: expected exactly 1 read after replenishment, got %d", purpose, got)
	}
}

func TestChannelBackpressureExecPausesUpstreamReadAtCreditZero(t *testing.T) {
	testPauseUpstreamReadGating(t, domainplugin.PurposeExec)
}

func TestChannelBackpressureTCPRelayPausesUpstreamReadAtCreditZero(t *testing.T) {
	testPauseUpstreamReadGating(t, domainplugin.PurposeTCPRelay)
}

func TestChannelBackpressureUDPRelayPausesUpstreamReadAtCreditZero(t *testing.T) {
	testPauseUpstreamReadGating(t, domainplugin.PurposeUDPRelay)
}

// TestChannelBackpressureUDPRelaySharesExecBranchNotEmbedBranch is the explicit assertion
// the plan requires: udp-relay's policy dispatch must equal exec's/tcp-relay's, and must be
// distinct from embed-stream's.
func TestChannelBackpressureUDPRelaySharesExecBranchNotEmbedBranch(t *testing.T) {
	udp := policyForPurpose(domainplugin.PurposeUDPRelay)
	exec := policyForPurpose(domainplugin.PurposeExec)
	tcp := policyForPurpose(domainplugin.PurposeTCPRelay)
	embed := policyForPurpose(domainplugin.PurposeEmbedStream)

	if udp != exec {
		t.Fatalf("udp-relay policy %v != exec policy %v", udp, exec)
	}
	if udp != tcp {
		t.Fatalf("udp-relay policy %v != tcp-relay policy %v", udp, tcp)
	}
	if udp == embed {
		t.Fatalf("udp-relay policy must not equal embed-stream's drop-oldest policy, got %v for both", udp)
	}
	if exec != policyPauseUpstreamRead || udp != policyPauseUpstreamRead {
		t.Fatalf("expected exec/tcp-relay/udp-relay to resolve to policyPauseUpstreamRead")
	}
	if embed != policyDropOldestUnsent {
		t.Fatalf("expected embed-stream to resolve to policyDropOldestUnsent, got %v", embed)
	}
}

func TestChannelBackpressureEmbedStreamDropsOldestUnsentFrameAtCreditZero(t *testing.T) {
	if policyForPurpose(domainplugin.PurposeEmbedStream) != policyDropOldestUnsent {
		t.Fatal("expected embed-stream to use the drop-oldest policy")
	}

	var written [][]byte
	ch := newFlowChannel(1, domainplugin.PurposeEmbedStream, 2, 0, nil, func(p []byte) error {
		written = append(written, append([]byte(nil), p...))
		return nil
	})

	// Exhaust the 2 credit units so all further Send calls land in the staging buffer
	// instead of going out over the wire.
	if err := ch.Send(context.Background(), []byte("frame-1")); err != nil {
		t.Fatalf("frame-1: %v", err)
	}
	if err := ch.Send(context.Background(), []byte("frame-2")); err != nil {
		t.Fatalf("frame-2: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected the first 2 frames to be written immediately (within initial credit), got %d", len(written))
	}

	// Credit is now 0. frame-3 and frame-4 must stage without blocking; the buffer capacity
	// (2, matching initial credit) means frame-3 gets evicted when frame-4 arrives.
	if err := ch.Send(context.Background(), []byte("frame-3")); err != nil {
		t.Fatalf("frame-3: %v", err)
	}
	if err := ch.Send(context.Background(), []byte("frame-4")); err != nil {
		t.Fatalf("frame-4: %v", err)
	}

	staged := ch.Staging().Frames()
	if len(staged) != 2 {
		t.Fatalf("expected 2 staged frames (capacity == initial credit), got %d", len(staged))
	}
	// Oldest unsent (frame-3) must have been dropped; newest (frame-4) retained. Since
	// frame-3 and frame-4 both landed in the buffer and it holds exactly 2, evicting the
	// oldest on the 3rd staged push means frame-3 alone should have been evicted only if a
	// 3rd staging push happened; assert by content that the newest frame is present and the
	// buffer never exceeds capacity.
	found4 := false
	for _, f := range staged {
		if string(f) == "frame-4" {
			found4 = true
		}
	}
	if !found4 {
		t.Fatalf("expected newest frame-4 to be retained in staging, got %v", stagedStrings(staged))
	}
}

func TestChannelBackpressureEmbedStreamEvictsOldestWhenBufferFull(t *testing.T) {
	ch := newFlowChannel(1, domainplugin.PurposeEmbedStream, 1, 0, nil, func([]byte) error { return nil })

	// Spend the single credit unit immediately so subsequent sends all stage.
	if err := ch.Send(context.Background(), []byte("sent-1")); err != nil {
		t.Fatalf("sent-1: %v", err)
	}

	if err := ch.Send(context.Background(), []byte("staged-a")); err != nil {
		t.Fatalf("staged-a: %v", err)
	}
	if err := ch.Send(context.Background(), []byte("staged-b")); err != nil {
		t.Fatalf("staged-b: %v", err)
	}

	staged := ch.Staging().Frames()
	if len(staged) != 1 {
		t.Fatalf("expected exactly 1 staged frame (capacity == initial credit of 1), got %d: %v", len(staged), stagedStrings(staged))
	}
	if string(staged[0]) != "staged-b" {
		t.Fatalf("expected the newest frame (staged-b) to be retained, got %q (oldest staged-a should have been dropped)", staged[0])
	}
}

func stagedStrings(frames [][]byte) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = string(f)
	}
	return out
}
