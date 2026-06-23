package wails

import (
	"context"
	"encoding/json"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"ssh-client/internal/usecase"
)

// PluginViewMessagePayload is emitted to the frontend when a plugin posts to a view.
type PluginViewMessagePayload struct {
	PluginID string          `json:"pluginId"`
	PanelID  string          `json:"panelId"`
	Message  json.RawMessage `json:"message"`
}

// PreparePluginViewPanel registers a WebView panel and returns a relay token.
func (a *AppAPI) PreparePluginViewPanel(pluginID, panelID string) (string, error) {
	if a.viewRelay == nil {
		return "", errPluginManagerUnavailable
	}
	return a.viewRelay.PreparePanel(pluginID, panelID)
}

// RelayPluginViewMessage forwards a UI message using a trusted relay token.
func (a *AppAPI) RelayPluginViewMessage(token string, message json.RawMessage) error {
	if a.viewRelay == nil {
		return errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return a.viewRelay.RelayMessage(ctx, token, message)
}

// ReleasePluginViewPanel invalidates a relay token and unregisters the panel.
func (a *AppAPI) ReleasePluginViewPanel(token string) {
	if a.viewRelay == nil {
		return
	}
	a.viewRelay.ReleasePanel(token)
}

// HandlePluginViewMessage implements usecase.PluginViewSink.
func (a *AppAPI) HandlePluginViewMessage(pluginID, panelID string, message json.RawMessage) error {
	if a.ctx == nil {
		return nil
	}
	wailsrt.EventsEmit(a.ctx, EventPluginViewMessage, PluginViewMessagePayload{
		PluginID: pluginID,
		PanelID:  panelID,
		Message:  message,
	})
	return nil
}

// SetPluginViewRelay binds the trusted view relay after composition.
func (a *AppAPI) SetPluginViewRelay(relay *usecase.PluginViewRelay) {
	a.viewRelay = relay
}
