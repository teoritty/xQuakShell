package plugin

import (
	"context"
	"fmt"
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/ipc"
)

// initializePluginProcess sends the initialize handshake. Compatibility was already resolved into
// `negotiated` (by Negotiate in Start, against the live registry) and any migration `warnings`
// collected there; this function only surfaces those advisories and deprecation notices — one per
// load — and sends the descriptor. Deprecated items still work (ADR-012); notices never block.
func initializePluginProcess(ctx context.Context, conn *ipc.Conn, plugin domainplugin.InstalledPlugin, dataDir string, portableReadOnly bool, negotiated domainplugin.NegotiatedDescriptor, warnings []string) error {
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	if portableReadOnly {
		slog.Warn("portable data root is read-only", "pluginId", plugin.Manifest.ID)
	}

	for _, w := range warnings {
		slog.Warn("plugin API advisory", "pluginId", plugin.Manifest.ID, "detail", w)
	}
	for _, n := range negotiated.DeprecationNotices(domainplugin.HostRegistry()) {
		slog.Warn("plugin uses deprecated API", "pluginId", plugin.Manifest.ID, "detail", n)
	}

	initParams := domainplugin.InitializeParams{
		PluginID:     plugin.Manifest.ID,
		APIVersion:   domainplugin.PluginAPIVersion,
		API:          domainplugin.HostDescriptor(),
		Capabilities: plugin.Manifest.Capabilities,
		DataDir:      dataDir,
		CoreVersion:  domainplugin.HostCoreVersion,
	}
	params, err := ipc.EncodeParams(initParams)
	if err != nil {
		return err
	}
	if _, err := conn.Call(initCtx, "initialize", params); err != nil {
		return fmt.Errorf("plugin initialize: %w", err)
	}
	return nil
}
