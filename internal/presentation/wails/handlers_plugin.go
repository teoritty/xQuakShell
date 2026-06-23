package wails

import (
	"context"
	"errors"
	"fmt"
	"time"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	domainplugin "ssh-client/internal/domain/plugin"
)

var errPluginManagerUnavailable = errors.New("plugin manager unavailable")

const pluginCallTimeout = 5 * time.Second

// ListPlugins returns discovered plugins and their runtime state.
func (a *AppAPI) ListPlugins() ([]PluginDTO, error) {
	if a.plugins == nil {
		return []PluginDTO{}, nil
	}
	list := a.plugins.List()
	result := make([]PluginDTO, 0, len(list))
	for _, info := range list {
		result = append(result, pluginInfoToDTO(info))
	}
	return result, nil
}

// PingPlugin sends a ping RPC to an already-running plugin process.
func (a *AppAPI) PingPlugin(pluginID string) (PluginPingResultDTO, error) {
	if a.plugins == nil {
		return PluginPingResultDTO{}, errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()

	result, err := a.plugins.Ping(ctx, pluginID)
	if err != nil {
		return PluginPingResultDTO{}, err
	}
	return PluginPingResultDTO{PluginID: pluginID, Result: result}, nil
}

// StartPlugin starts a plugin process for development or settings (does not send activate).
func (a *AppAPI) StartPlugin(pluginID string) error {
	if a.plugins == nil {
		return errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return a.plugins.StartPluginManual(ctx, pluginID)
}

// SetPluginEnabled toggles whether a plugin is allowed to run.
func (a *AppAPI) SetPluginEnabled(pluginID string, enabled bool) error {
	if a.plugins == nil {
		return errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	if err := a.plugins.SetPluginEnabled(ctx, pluginID, enabled); err != nil {
		return err
	}
	if !enabled {
		_ = a.plugins.StopPlugin(ctx, pluginID)
	}
	a.EmitPluginContributionsChanged()
	return nil
}

// PluginStateChangedPayload is emitted when plugin runtime state changes.
type PluginStateChangedPayload struct {
	PluginID  string `json:"pluginId"`
	State     string `json:"state"`
	SessionID string `json:"sessionId,omitempty"`
}

// EmitPluginStateChanged notifies the frontend of plugin lifecycle changes.
func (a *AppAPI) EmitPluginStateChanged(pluginID, state, sessionID string) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventPluginStateChanged, PluginStateChangedPayload{
		PluginID:  pluginID,
		State:     state,
		SessionID: sessionID,
	})
}

// EmitPluginContributionsChanged notifies the frontend to refresh merged contributions.
func (a *AppAPI) EmitPluginContributionsChanged() {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventPluginContributionsChanged, struct{}{})
}

// SelectPluginSourceDir opens a directory picker for plugin installation.
func (a *AppAPI) SelectPluginSourceDir() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenDirectoryDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select Plugin Folder",
	})
}

// SelectPluginBundleFile opens a file picker for .xqs-plugin bundles.
func (a *AppAPI) SelectPluginBundleFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("no wails context")
	}
	return wailsrt.OpenFileDialog(a.ctx, wailsrt.OpenDialogOptions{
		Title: "Select Plugin Bundle",
		Filters: []wailsrt.FileFilter{
			{DisplayName: "xQuakShell Plugin (*.xqs-plugin)", Pattern: "*.xqs-plugin"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
}

// PreviewPluginInstall validates a plugin folder or bundle and returns permission summary.
func (a *AppAPI) PreviewPluginInstall(sourcePath string) (PluginInstallPreviewDTO, error) {
	if a.plugins == nil {
		return PluginInstallPreviewDTO{}, errPluginManagerUnavailable
	}
	policy, err := a.pluginTrustPolicy()
	if err != nil {
		return PluginInstallPreviewDTO{}, err
	}
	preview, err := a.plugins.PreviewInstall(sourcePath, policy)
	if err != nil {
		return PluginInstallPreviewDTO{}, err
	}
	return previewToDTO(preview), nil
}

// InstallPlugin copies a plugin bundle into user storage after user consent.
func (a *AppAPI) InstallPlugin(sourcePath string, grantSecretAccess bool, grantMultiSessionAccess bool) (PluginDTO, error) {
	if a.plugins == nil {
		return PluginDTO{}, errPluginManagerUnavailable
	}
	policy, err := a.pluginTrustPolicy()
	if err != nil {
		return PluginDTO{}, err
	}
	preview, err := a.plugins.PreviewInstall(sourcePath, policy)
	if err != nil {
		return PluginDTO{}, err
	}
	if preview.RequiresSecretAccess && !grantSecretAccess {
		return PluginDTO{}, fmt.Errorf("secret access consent required for this plugin")
	}
	if preview.MultiSessionWarning && !grantMultiSessionAccess {
		return PluginDTO{}, fmt.Errorf("multi-session consent required for this plugin")
	}
	installed, err := a.plugins.Install(sourcePath, policy, grantMultiSessionAccess)
	if err != nil {
		return PluginDTO{}, err
	}
	if preview.RequiresSecretAccess && grantSecretAccess && a.pluginVaultGrant != nil {
		if err := a.pluginVaultGrant(installed.Manifest.ID); err != nil {
			return PluginDTO{}, err
		}
	}
	if preview.MultiSessionWarning && grantMultiSessionAccess && a.pluginMultiSessionGrant != nil {
		if err := a.pluginMultiSessionGrant(installed.Manifest.ID); err != nil {
			return PluginDTO{}, err
		}
	}
	for _, info := range a.plugins.List() {
		if info.ID == installed.Manifest.ID {
			return pluginInfoToDTO(info), nil
		}
	}
	return pluginDTOFromInstalled(installed), nil
}

func pluginDTOFromInstalled(installed domainplugin.InstalledPlugin) PluginDTO {
	return PluginDTO{
		ID:                   installed.Manifest.ID,
		Name:                 installed.Manifest.Name,
		Version:              installed.Manifest.Version,
		Description:          installed.Manifest.Description,
		Source:               string(installed.Source),
		State:                "discovered",
		RequiresSecretAccess: installed.Manifest.RequiresSecretAccess(),
		Signed:               installed.Manifest.Signature != "",
		Enabled:              true,
	}
}
