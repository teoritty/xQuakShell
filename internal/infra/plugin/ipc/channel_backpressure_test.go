package ipc

import (
	"context"
	"sync"
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

	if err := credit.ReplenishOutbound(1, testCreditCeiling); err != nil {
		t.Fatalf("purpose %q: replenish: %v", purpose, err)
	}

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

func TestChannelBackpressureEmbedStreamPausesUpstreamReadAtCreditZero(t *testing.T) {
	testPauseUpstreamReadGating(t, domainplugin.PurposeEmbedStream)
}

// TestChannelBackpressureEmbedStreamBlocksRatherThanDroppingInput is the regression for the
// drop-oldest policy embed-stream used to carry. Its outbound direction is browser control
// input: every event is a state transition, so a dropped KeyEvent with down=0 leaves a key
// held down on the remote machine with nothing in any log. Send must block for credit like
// every other purpose, and must never silently succeed having discarded the frame.
func TestChannelBackpressureEmbedStreamBlocksRatherThanDroppingInput(t *testing.T) {
	var mu sync.Mutex
	var written [][]byte
	const initialCredit = 1
	ch := newFlowChannel(1, domainplugin.PurposeEmbedStream, initialCredit, 0, nil, func(p []byte) error {
		mu.Lock()
		written = append(written, append([]byte(nil), p...))
		mu.Unlock()
		return nil
	})

	// key-down consumes the only credit unit.
	if err := ch.Send(context.Background(), []byte("key-down")); err != nil {
		t.Fatalf("key-down: %v", err)
	}

	// key-up finds credit at 0. The old policy returned nil here having thrown the frame away,
	// stranding the key held down forever.
	sendDone := make(chan error, 1)
	go func() { sendDone <- ch.Send(context.Background(), []byte("key-up")) }()

	select {
	case err := <-sendDone:
		t.Fatalf("key-up returned (err=%v) while credit was 0 instead of waiting; the frame was dropped", err)
	case <-time.After(100 * time.Millisecond):
	}

	mu.Lock()
	got := len(written)
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected only key-down on the wire while credit is 0, got %d frames", got)
	}

	// Once the plugin returns credit, the held frame goes out — delivered late, never lost.
	if err := ch.ReceiveCredit(1); err != nil {
		t.Fatalf("replenish: %v", err)
	}
	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatalf("key-up after replenish: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("key-up never unblocked after credit replenishment")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(written) != 2 || string(written[1]) != "key-up" {
		t.Fatalf("expected key-up to reach the wire after replenishment, got %v", written)
	}
}

func stagedStrings(frames [][]byte) []string {
	out := make([]string, len(frames))
	for i, f := range frames {
		out[i] = string(f)
	}
	return out
}
