package plugin

import (
	"context"
	"encoding/json"
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

// ChannelHandle is a domain-level reference to a host-owned channel bus channel, passed to a
// ChannelPurposeBackend so it can wire the far end without depending on the ipc frame layer.
type ChannelHandle struct {
	ChannelID       uint32
	PluginID        string
	Purpose         string
	ParentSessionID string
	Hint            string
}

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
