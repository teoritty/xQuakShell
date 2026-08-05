package main

import (
	presentation "xquakshell/internal/presentation/wails"
)

// Wails bindings for what a plugin draws: discovery subtrees (ADR-014) and the tabs, dialogs and
// node panels of ADR-015.
//
// Same facade, same one-line delegates as app.go — split off because app.go had become the place
// every feature adds two methods to, and these belong to one story that can be read on its own.
// Being in package main is what matters: app_bindings_test.go still sees them, and Wails still
// binds them.

// Discovery subtrees (ADR-014). Everything here is addressed by connectionId; the sessionId a
// plugin is actually reached through is resolved in the backend and never crosses this boundary.

func (a *App) GetDiscoveryTree(connectionId string) (presentation.DiscoverySnapshotDTO, error) {
	return a.api.GetDiscoveryTree(connectionId)
}

func (a *App) SetDiscoveryObserved(connectionId string, nodeIds []string) error {
	return a.api.SetDiscoveryObserved(connectionId, nodeIds)
}

// InvokeDiscoveryAction names the addressee plugin explicitly: node ids are unique only within one
// plugin's own subtree.
func (a *App) InvokeDiscoveryAction(connectionId string, pluginId string, nodeIds []string, actionId string) error {
	return a.api.InvokeDiscoveryAction(connectionId, pluginId, nodeIds, actionId)
}

// CloseSurface closes a plugin-owned tab. A surface is addressed by its own id and never by a
// session id: the session is how the host reaches the plugin, not how the UI names a tab
// (ADR-015).
func (a *App) CloseSurface(surfaceId string) error {
	return a.api.CloseSurface(surfaceId)
}

// SendSurfaceInput forwards keystrokes to the plugin owning an interactive surface.
func (a *App) SendSurfaceInput(surfaceId string, dataBase64 string) error {
	return a.api.SendSurfaceInput(surfaceId, dataBase64)
}

// ResizeSurface reports a surface's new character grid.
func (a *App) ResizeSurface(surfaceId string, cols int, rows int) error {
	return a.api.ResizeSurface(surfaceId, cols, rows)
}

// SubmitPluginDialog delivers a plugin dialog's answer. A validation failure comes back as an
// error the modal shows in place, so the dialog stays open and the user corrects the field.
func (a *App) SubmitPluginDialog(dialogId string, values map[string]string) error {
	return a.api.SubmitPluginDialog(dialogId, values)
}

// CancelPluginDialog closes a plugin dialog without an answer.
func (a *App) CancelPluginDialog(dialogId string) error {
	return a.api.CancelPluginDialog(dialogId)
}

// DescribeDiscoveryNode returns a discovery node's property panel. pluginId is named explicitly
// for the same reason InvokeDiscoveryAction names it: node ids are unique only within one plugin's
// own subtree.
func (a *App) DescribeDiscoveryNode(connectionId string, pluginId string, nodeId string) (presentation.NodeDetailsDTO, error) {
	return a.api.DescribeDiscoveryNode(connectionId, pluginId, nodeId)
}

// ApplyDiscoveryNodeDetails hands edited values to the node's owner, which persists them.
func (a *App) ApplyDiscoveryNodeDetails(connectionId string, pluginId string, nodeId string, values map[string]string) error {
	return a.api.ApplyDiscoveryNodeDetails(connectionId, pluginId, nodeId, values)
}
