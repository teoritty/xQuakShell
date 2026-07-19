package ipc

import (
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// channelMux demultiplexes kind=0x02/0x03 frames by channelId to the owning channel.
// Routing and channel construction — no policy of its own: credit rules live in channelCredit,
// purpose-backend wiring in the capability layer.
// defaultClosedChannelGrace is how long a closed channel stays tracked so that frames the
// plugin already put on the wire land as no-ops rather than protocol violations. It only has to
// outlast bytes in flight on a pipe, not any plugin-side timeout.
const defaultClosedChannelGrace = 30 * time.Second

type channelMux struct {
	writeBinary func(channelID uint32, payload []byte) error
	closedGrace time.Duration

	mu       sync.Mutex
	channels map[uint32]*channel
	// releaseTimers holds the pending removal of each closed-but-still-tracked channel, so
	// connection teardown can cancel them instead of firing into a dead mux.
	releaseTimers map[uint32]*time.Timer
}

// newChannelMux builds a mux whose channels emit outbound data through writeBinary (Conn's
// serialized frame writer in production, a fake in tests).
func newChannelMux(writeBinary func(channelID uint32, payload []byte) error) *channelMux {
	return &channelMux{
		writeBinary:   writeBinary,
		closedGrace:   defaultClosedChannelGrace,
		channels:      make(map[uint32]*channel),
		releaseTimers: make(map[uint32]*time.Timer),
	}
}

// CloseAndRelease closes a channel and schedules it to be dropped from the mux once the grace
// period expires.
//
// The two steps are deliberately separated in time. Closing at once is required: it unparks the
// backend pumps sitting in Recv, which would otherwise never exit. Dropping at once is wrong:
// the plugin may still have frames on the wire for this id, and an unknown id is fatal to the
// connection (ADR-011 §2a) — so an immediate drop would kill a well-behaved plugin for the
// host's own decision to close. Keeping it tracked forever is the other extreme: one map entry
// per channel ever opened, which for a long VNC session is a leak. The grace period is the seam
// between those two, and it is why channel.deliver treats a closed-but-tracked channel as a
// silent no-op rather than an error.
func (m *channelMux) CloseAndRelease(id uint32) {
	ch, ok := m.Get(id)
	if !ok {
		return
	}
	ch.Close()

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, pending := m.releaseTimers[id]; pending {
		return
	}
	m.releaseTimers[id] = time.AfterFunc(m.closedGrace, func() {
		m.mu.Lock()
		delete(m.channels, id)
		delete(m.releaseTimers, id)
		m.mu.Unlock()
	})
}

// ReleaseAll closes and drops every channel, cancelling any pending grace timers. Used at
// connection teardown, where there is no longer a plugin whose in-flight frames need sparing.
func (m *channelMux) ReleaseAll() {
	m.mu.Lock()
	channels := m.channels
	timers := m.releaseTimers
	m.channels = make(map[uint32]*channel)
	m.releaseTimers = make(map[uint32]*time.Timer)
	m.mu.Unlock()

	for _, t := range timers {
		t.Stop()
	}
	for _, ch := range channels {
		ch.Close()
	}
}

// Register creates and tracks a new channel for id. The caller (the channel capability proxy,
// via Conn.OpenDataPath) is responsible for allocating unique, monotonic ids.
//
// Every registered channel is flow-controlled: this is the only place a channel is constructed,
// so there is no way to obtain one whose credit window, throttle or backpressure gate is
// absent. An unrestricted variant used to exist for callers that had no purpose to hand, and
// since it was the one production actually reached, the entire flow-control layer was dead.
func (m *channelMux) Register(id uint32, purpose string, throughputKbps int) *channel {
	ch := newFlowChannel(id, purpose, domainplugin.InitialCredit(purpose), throughputKbps, nil, func(payload []byte) error {
		if m.writeBinary == nil {
			return errNoChannelWriter
		}
		return m.writeBinary(id, payload)
	})
	m.mu.Lock()
	m.channels[id] = ch
	m.mu.Unlock()
	return ch
}

// MarkOpened releases the outbound send gate for a channel once its channel.open reply has
// reached the wire (see channel.opened). A no-op for an unknown id: a reply for a channel the
// mux never registered means the open failed, and there is nothing to release.
func (m *channelMux) MarkOpened(id uint32) {
	if ch, ok := m.Get(id); ok {
		ch.markOpened()
	}
}

// Get returns the channel for id, if the mux still tracks it (open or already closed).
func (m *channelMux) Get(id uint32) (*channel, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.channels[id]
	return ch, ok
}

// Dispatch routes one kind=0x02/0x03 frame to its channel. A frame for a channelId the
// mux has never registered (or has fully removed) is a protocol violation per ADR-011
// §2a — fail-fast, not a silent drop. A frame for a channel that is merely Close()d but
// still tracked is routed through and dropped as a no-op by channel.deliver.
//
// Routing is by channelId *and* kind: data (0x02) reaches the channel's consumer queue,
// credit (0x03) reaches its outbound window. Both are routing decisions — the credit window's
// own policy (ceiling, id agreement) stays in channel/channelCredit, not here.
func (m *channelMux) Dispatch(hdr FrameHeader, payload []byte) error {
	ch, ok := m.Get(hdr.ChannelID)
	if !ok {
		return newProtocolViolation("frame kind 0x%02x for unknown channel id %d", hdr.Kind, hdr.ChannelID)
	}

	switch hdr.Kind {
	case domainplugin.FrameKindBinary:
		if err := ch.deliver(hdr.Kind, payload); err != nil {
			return newProtocolViolation("channel %d: %v", hdr.ChannelID, err)
		}
		return nil
	case domainplugin.FrameKindCredit:
		return ch.receiveCreditFrame(hdr.ChannelID, payload)
	default:
		// Unreachable: validateFrameHeader rejects any other kind before a frame is read,
		// and readLoop only routes non-JSON-RPC frames here. Kept so that adding a frame
		// kind cannot silently fall through into the data queue.
		return newProtocolViolation("frame kind 0x%02x is not routable to a channel", hdr.Kind)
	}
}
