package plugin

import (
	"context"
	"encoding/json"
)

// SurfaceInboundPort handles plugin->host surface.* RPC, mirroring ChannelInboundPort and
// DiscoveryInboundPort: the ipc layer knows how to decode a frame but nothing about which session
// a plugin holds, and the usecase layer knows that but must not import the transport.
type SurfaceInboundPort interface {
	Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error)
}

// SurfaceOutboundPort delivers host->plugin surface notifications. Every method is one-way by
// design: input, a resize and a close are all facts about what already happened on screen, and
// there is nothing the plugin could usefully answer.
type SurfaceOutboundPort interface {
	// Input carries bytes the user typed. Sent only for interactive surface kinds.
	Input(pluginID, surfaceID string, data []byte)
	// Resize reports the surface's new character grid. Sent only for interactive kinds.
	Resize(pluginID, surfaceID string, cols, rows uint16)
	// Closed reports that the surface is gone for a reason the plugin did not cause: the user
	// closed the tab, or the parent session ended. A plugin that closed the surface itself is not
	// told again.
	Closed(pluginID, surfaceID, reason string)
}

// SurfaceSessionCloser tears down every surface bound to a session. It is the surface counterpart
// of ChannelSessionCloser and is called from the same step of the session close sequence, before
// the SSH client itself goes away (ADR-011 §Session lifecycle coupling, ADR-015 §1).
type SurfaceSessionCloser interface {
	CloseSurfacesForSession(sessionID string)
}

// SurfacePluginCloser tears down every surface owned by a plugin process. Called when the process
// exits or crashes, independently of whether the parent session is still alive: a tab whose
// producer is gone shows a stream nobody is writing to.
type SurfacePluginCloser interface {
	CloseSurfacesForPlugin(pluginID string)
}
