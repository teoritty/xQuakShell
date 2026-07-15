package plugin

import (
	"context"
	"fmt"
	"log/slog"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/ipc"
)

func initializePluginProcess(ctx context.Context, conn *ipc.Conn, plugin domainplugin.InstalledPlugin, dataDir string, portableReadOnly bool) error {
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	if portableReadOnly {
		slog.Warn("portable data root is read-only", "pluginId", plugin.Manifest.ID)
	}

	// Re-verify the plugin's declared requirements against the LIVE registry before handing it a
	// connection. The manifest was already checked at install time, but the host build may have
	// changed since (downgrade, dev build, stale snapshot) — the handshake is the source of truth,
	// and we fail closed on any skew rather than start an incompatible plugin (ADR-012 edge #11).
	eff, _, err := domainplugin.EffectiveRequirements(&plugin.Manifest)
	if err != nil {
		return fmt.Errorf("plugin initialize: %w", err)
	}
	if report := eff.CheckAgainstHost(domainplugin.HostRegistry()); report != nil {
		return fmt.Errorf("plugin initialize: %w", report)
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
