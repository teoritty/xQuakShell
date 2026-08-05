package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Host->plugin surface notification methods (ADR-015 §1). All three are notifications: input, a
// resize and a close are facts about what already happened on screen, and there is nothing a
// plugin could usefully answer.
const (
	surfaceInputMethod  = "surface.input"
	surfaceResizeMethod = "surface.resize"
	surfaceClosedMethod = "surface.closed"
)

// SurfaceNotifier turns the outbound port into plugin notifications, the same shape
// DiscoveryObserver uses for discovery.observe. It is a translation layer and holds no state.
type SurfaceNotifier struct {
	notifier DiscoveryNotifier
}

// NewSurfaceNotifier wires the outbound half. The parameter is DiscoveryNotifier because that
// interface is already exactly "send a host->plugin notification" and is satisfied by
// *PluginManager; giving it a second name would suggest the two differ.
func NewSurfaceNotifier(notifier DiscoveryNotifier) *SurfaceNotifier {
	return &SurfaceNotifier{notifier: notifier}
}

// Input forwards user keystrokes. Base64 for the same reason session.writeInput uses it: the
// payload is arbitrary bytes and JSON has no way to carry those.
func (n *SurfaceNotifier) Input(pluginID, surfaceID string, data []byte) {
	n.send(pluginID, surfaceInputMethod, map[string]string{
		"surfaceId":  surfaceID,
		"dataBase64": base64.StdEncoding.EncodeToString(data),
	})
}

// Resize forwards the surface's new character grid.
func (n *SurfaceNotifier) Resize(pluginID, surfaceID string, cols, rows uint16) {
	n.send(pluginID, surfaceResizeMethod, map[string]any{
		"surfaceId": surfaceID,
		"cols":      cols,
		"rows":      rows,
	})
}

// Closed tells a plugin its surface is gone for a reason it did not cause.
func (n *SurfaceNotifier) Closed(pluginID, surfaceID, reason string) {
	n.send(pluginID, surfaceClosedMethod, map[string]string{
		"surfaceId": surfaceID,
		"reason":    reason,
	})
}

// send marshals and dispatches, logging rather than returning failures.
//
// There is nothing a caller could do with the error. Every one of these is a notification about
// something that has already happened in the UI, sent to a process that may be mid-restart; the
// surface state the host holds is already correct, and retrying would only re-announce a fact.
func (n *SurfaceNotifier) send(pluginID, method string, payload any) {
	if n.notifier == nil {
		return
	}
	params, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("surface: marshal notification failed", "method", method, "err", err)
		return
	}
	if err := n.notifier.Notify(context.Background(), pluginID, method, params); err != nil {
		slog.Debug("surface: notify failed", "method", method, "pluginId", pluginID, "err", err)
	}
}

var _ domainplugin.SurfaceOutboundPort = (*SurfaceNotifier)(nil)
