package ipc

import (
	"context"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

// exhaustionPolicy is the per-purpose ADR-011 §2b behavior when a channel's outbound credit
// hits zero.
type exhaustionPolicy int

const (
	// policyPauseUpstreamRead covers exec/tcp-relay/udp-relay: the backend's own upstream
	// read loop must stop pulling more data until credit frees up, so backpressure
	// propagates to the real source (SSH stdout pipe / relayed socket) instead of an
	// in-process buffer growing without bound.
	policyPauseUpstreamRead exhaustionPolicy = iota
	// policyDropOldestUnsent covers embed-stream: latest-frame-wins: the newest frame
	// evicts the oldest still-unsent one. There is no upstream to pause — this is host-side
	// buffer logic only.
	policyDropOldestUnsent
)

// policyForPurpose dispatches the ADR-011 §2b exhaustion policy for a channel purpose.
// udp-relay deliberately shares exec/tcp-relay's pause-upstream-read branch, not
// embed-stream's drop-oldest branch: at credit 0 the UDP backend stops reading its socket
// and the OS receive buffer bounds and drops excess datagrams, which is the correct
// (bounded, no-unbounded-queue) UDP behavior rather than a distinct policy.
func policyForPurpose(purpose string) exhaustionPolicy {
	if purpose == domainplugin.PurposeEmbedStream {
		return policyDropOldestUnsent
	}
	return policyPauseUpstreamRead
}

// backendGate is the capacity signal a pause-upstream-read purpose backend's read loop
// blocks on before pulling more data from its upstream source. It never itself queues
// anything — it only reports "credit is available", derived from the channel's live
// outbound credit — so no unbounded in-process buffer can grow behind it.
type backendGate struct {
	credit *channelCredit
}

func newBackendGate(credit *channelCredit) *backendGate {
	return &backendGate{credit: credit}
}

// WaitForCapacity blocks until outbound credit is available, or ctx is done. It does not
// consume credit: consumption happens at actual send time (channel.Send), keeping this a
// pure "may I proceed" signal.
func (g *backendGate) WaitForCapacity(ctx context.Context) error {
	return g.credit.WaitOutboundAvailable(ctx)
}

// stagingBuffer implements the embed-stream drop-oldest-unsent-frame policy: a bounded,
// host-side buffer of frames waiting for outbound credit. When a new frame arrives and the
// buffer is already at capacity, the oldest entry is evicted first (latest-frame-wins).
type stagingBuffer struct {
	mu       sync.Mutex
	capacity int
	frames   [][]byte
}

func newStagingBuffer(capacity int) *stagingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &stagingBuffer{capacity: capacity}
}

// Push appends payload, evicting the oldest still-unsent frame first if the buffer is
// already at capacity.
func (b *stagingBuffer) Push(payload []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.frames) >= b.capacity {
		b.frames = b.frames[1:]
	}
	b.frames = append(b.frames, append([]byte(nil), payload...))
}

// Frames returns a snapshot of the currently staged frames, oldest first.
func (b *stagingBuffer) Frames() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, len(b.frames))
	copy(out, b.frames)
	return out
}

// Len reports the number of currently staged frames.
func (b *stagingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.frames)
}
