package ipc

import "sync"

// channelMux demultiplexes kind=0x02/0x03 frames by channelId to the owning channel.
// Routing only — no policy, no credit logic (Stage 5), no purpose-backend wiring (Stage 3).
type channelMux struct {
	mu       sync.Mutex
	channels map[uint32]*channel
}

func newChannelMux() *channelMux {
	return &channelMux{channels: make(map[uint32]*channel)}
}

// Register creates and tracks a new channel for id. The caller (Stage 3's ChannelProxy)
// is responsible for allocating unique, monotonic ids.
func (m *channelMux) Register(id uint32) *channel {
	ch := newChannel(id)
	m.mu.Lock()
	m.channels[id] = ch
	m.mu.Unlock()
	return ch
}

// Get returns the channel for id, if the mux still tracks it (open or already closed).
func (m *channelMux) Get(id uint32) (*channel, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.channels[id]
	return ch, ok
}

// Remove drops a channel from the mux entirely. Any subsequent frame for id becomes a
// protocol violation via Dispatch, distinct from a locally-closed-but-still-tracked
// channel, which silently no-ops inbound frames instead.
func (m *channelMux) Remove(id uint32) {
	m.mu.Lock()
	delete(m.channels, id)
	m.mu.Unlock()
}

// Dispatch routes one kind=0x02/0x03 frame to its channel. A frame for a channelId the
// mux has never registered (or has fully removed) is a protocol violation per ADR-011
// §2a — fail-fast, not a silent drop. A frame for a channel that is merely Close()d but
// still tracked is routed through and dropped as a no-op by channel.deliver.
func (m *channelMux) Dispatch(hdr FrameHeader, payload []byte) error {
	ch, ok := m.Get(hdr.ChannelID)
	if !ok {
		return newProtocolViolation("frame kind 0x%02x for unknown channel id %d", hdr.Kind, hdr.ChannelID)
	}
	if err := ch.deliver(hdr.Kind, payload); err != nil {
		return newProtocolViolation("channel %d: %v", hdr.ChannelID, err)
	}
	return nil
}
