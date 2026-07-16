package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

// ChannelInboundPort handles plugin->core channel.open/channel.close RPC with usecase-level
// routing, mirroring TunnelInboundPort/embed's late-bound adapter pattern.
type ChannelInboundPort interface {
	Open(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
	Close(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
}

// ChannelSessionCloser closes every channel bound to a parentSessionId. Implemented by the
// composition-root channel bus (internal/infra/plugin/capability) and invoked synchronously by
// SessionLifecycleService.CloseSession before the session's ssh client is closed, so exec
// channels (which ride that client) never outlive the session that owns them (ADR-011 §Session
// lifecycle coupling).
type ChannelSessionCloser interface {
	CloseSession(sessionID string)
}

// ChannelDataPath is the minimal read/write/flow-control surface a purpose backend's Wire
// method uses to move bytes across an already-open channel. It is implemented by the host's
// real per-channel ipc.channel (via a thin composition-root adapter) and by fakes in tests,
// kept as a domain-level interface so purpose backends never import the ipc frame layer.
type ChannelDataPath interface {
	// Send emits one outbound kind=0x02 frame, blocking per the channel's ADR-011 §2b
	// exhaustion policy for the channel's purpose.
	Send(ctx context.Context, payload []byte) error
	// Recv blocks until an inbound frame (plugin -> host) is available or the channel is
	// closed; ok is false only once the channel is closed with nothing left queued.
	//
	// Recv does not return credit to the plugin: receiving a frame only means the host took
	// it off the wire, which says nothing about whether its consumer has caught up. Ack does
	// that, and only once the frame has actually reached the consumer.
	Recv() ([]byte, bool)
	// WaitForCapacity blocks until outbound credit is available or ctx is done — the
	// pause-upstream-read signal a backend's own upstream read loop must gate on before
	// pulling more data from its source (ADR-011 §2b).
	WaitForCapacity(ctx context.Context) error
	// Ack returns one unit of credit to the plugin, reporting that the frame from the
	// preceding Recv has been handed to its consumer. It is the caller's obligation, not a
	// side effect of Recv: only the backend knows when a frame is genuinely consumed rather
	// than merely received, and that is exactly what the plugin's send window must track.
	//
	// A backend that never Acks stalls its own channel once the window drains. That is
	// deliberate: an un-acked frame means an un-consumed frame, and the plugin must not run
	// further ahead of a consumer that is not keeping up.
	Ack(ctx context.Context) error
	// Close releases the channel and unblocks anything parked in Recv, Send or
	// WaitForCapacity. Idempotent.
	Close() error
}

// ChannelDataPathOpener opens the real data path for an already-authorized channel. It is
// implemented by the ipc layer's Conn and handed to the channel capability proxy by the
// composition root, so the proxy can furnish every ChannelHandle with a live data path
// without itself depending on the frame transport.
type ChannelDataPathOpener interface {
	OpenDataPath(id uint32, purpose string) (ChannelDataPath, error)
}

// ErrNilChannelDataPath is returned by NewChannelHandle when no data path is supplied. A
// channel without one can be authorized, audited and assigned an id, yet cannot move a single
// byte — the failure this type exists to make unrepresentable.
var ErrNilChannelDataPath = errors.New("plugin: channel handle requires a data path")

// ChannelHandle is a domain-level reference to a host-owned channel bus channel, passed to a
// ChannelPurposeBackend so it can wire the far end without depending on the ipc frame layer.
//
// Its fields are unexported and reachable only through NewChannelHandle so that a handle
// without a data path cannot be constructed at all. The alternative — an exported struct plus
// a nil check in every backend — is what let the bus ship wired to nothing: each backend
// dutifully checked, found nil, and returned success.
type ChannelHandle struct {
	channelID       uint32
	pluginID        string
	purpose         string
	parentSessionID string
	hint            string
	data            ChannelDataPath
}

// NewChannelHandle builds a handle for an open channel. data must be non-nil.
func NewChannelHandle(channelID uint32, pluginID, purpose, parentSessionID, hint string, data ChannelDataPath) (*ChannelHandle, error) {
	if data == nil {
		return nil, ErrNilChannelDataPath
	}
	return &ChannelHandle{
		channelID:       channelID,
		pluginID:        pluginID,
		purpose:         purpose,
		parentSessionID: parentSessionID,
		hint:            hint,
		data:            data,
	}, nil
}

// ChannelID is the host-allocated, never-reused id this channel is addressed by.
func (h *ChannelHandle) ChannelID() uint32 { return h.channelID }

// PluginID is the plugin process that owns this channel.
func (h *ChannelHandle) PluginID() string { return h.pluginID }

// Purpose is the manifest-declared purpose this channel was opened for.
func (h *ChannelHandle) Purpose() string { return h.purpose }

// ParentSessionID is the session this channel is bound to and cascades closed with.
func (h *ChannelHandle) ParentSessionID() string { return h.parentSessionID }

// Hint is the purpose-specific target the plugin requested (dial target, tunnelId, ...).
func (h *ChannelHandle) Hint() string { return h.hint }

// Data is the channel's live data path. Never nil.
func (h *ChannelHandle) Data() ChannelDataPath { return h.data }

// ChannelPurposeBackend is the contract each purpose (exec/tcp-relay/embed-stream, implemented
// in Stages 6-8) satisfies to authorize and wire a channel's host-owned far end.
type ChannelPurposeBackend interface {
	// Authorize validates the request before any channel resource (id, remote process, dial) is
	// created. Must run after the maxConcurrent slot is reserved but before anything is wired.
	Authorize(purpose string, parentSessionID string, hint string) error
	// Wire connects the host-owned far end once the channel id is allocated and the slot is
	// reserved. Only on success does the caller commit channel ownership.
	Wire(ctx context.Context, ch *ChannelHandle) error
	// CloseRemote tears down the host-owned far end (SSH exec channel / relay conn / embed
	// wire). Invoked by an explicit channel.close, by session-lifecycle cascade close, and by
	// plugin-process-crash teardown, so a remote process can never outlive its channel.
	CloseRemote() error
}
