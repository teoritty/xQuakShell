package plugin

import (
	"context"
	"encoding/json"
)

// DiscoveryInboundPort handles plugin->core discovery.publish RPC with usecase-level routing,
// mirroring ChannelInboundPort: the ipc layer knows how to decode a request but nothing about
// which connection a sessionId belongs to, and the usecase layer knows the tree but must not
// import the transport.
//
// Only publish appears here. observe and invokeAction travel host->plugin and therefore need no
// inbound port at all — the host simply never addresses them to a plugin that lacks the
// capability (ADR-014 "Security model"), which is an addressing decision, not a gate.
type DiscoveryInboundPort interface {
	Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error)
}

// DiscoveryConnectionClearer drops a connection's whole discovery tree, the counterpart of
// ChannelSessionCloser for discovery state.
//
// It is keyed by connectionID, not sessionID, because one connection shows exactly one subtree
// no matter how many tabs are open (ADR-014 "Leading session"). Closing one session of several
// is a leader handover, not a clear; only the loss of the last ready session reaches here.
type DiscoveryConnectionClearer interface {
	ClearConnection(connectionID string)
}
