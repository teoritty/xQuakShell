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
	eff, warnings, err := domainplugin.EffectiveRequirements(&plugin.Manifest)
	if err != nil {
		return fmt.Errorf("plugin initialize: %w", err)
	}
	registry := domainplugin.HostRegistry()
	if report := eff.CheckAgainstHost(registry); report != nil {
		return fmt.Errorf("plugin initialize: %w", report)
	}

	// Surface migration/advisory warnings and deprecation notices once per load so authors can
	// migrate ahead of removal. Deprecated items still work (ADR-012); these never block startup.
	for _, w := range warnings {
		slog.Warn("plugin API advisory", "pluginId", plugin.Manifest.ID, "detail", w)
	}
	for _, n := range eff.DeprecationNotices(registry) {
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
