package usecase

import (
	"fmt"
	"log/slog"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// InstallPreview describes a plugin before installation.
type InstallPreview struct {
	ID                           string
	Name                         string
	Version                      string
	Description                  string
	Signed                       bool
	SignatureVerified            bool
	ChecksumPresent              bool
	RequiresSecretAccess         bool
	RequiresAuthProviderAccess   bool
	RequiresTunnelProviderAccess bool
	MultiSessionWarning          bool
	ArbitraryNetworkWarning      bool
	ExecAccessWarning            bool
	UnsignedWarning              bool
	UntrustedSignatureWarning    bool
	Permissions                  []string
}

// BundleLoader loads a plugin directory or bundle for preview/install.
type BundleLoader func(sourcePath string) (domainplugin.InstalledPlugin, error)

// BundleInstaller copies a plugin into portable storage next to the executable.
type BundleInstaller func(sourcePath, dataRoot string) (domainplugin.InstalledPlugin, error)

// PluginManagerConfig wires optional install helpers from the composition root.
type PluginManagerConfig struct {
	Registry       *PluginRegistry
	Host           domainplugin.ProcessHost
	LoadBundle     BundleLoader
	InstallBundle  BundleInstaller
	InstallRoot    string
	PortableData   domain.PortableDataStore
	Bundle         domainplugin.BundlePort
	Portable       domain.PortableRuntime
	SettingsReader PluginSettingsReader
	PluginSettings *PluginVaultSettings
	StartAudit     PluginStartAuditFunc
}

// NewPluginManagerWithConfig creates a plugin manager with install support.
func NewPluginManagerWithConfig(cfg PluginManagerConfig) *PluginManager {
	if cfg.InstallRoot == "" {
		panic("plugin manager: InstallRoot is required")
	}
	m := &PluginManager{
		registry:       cfg.Registry,
		host:           cfg.Host,
		sessionCounts:  make(map[string]int),
		lastActivity:   make(map[string]time.Time),
		loadBundle:     cfg.LoadBundle,
		installBundle:  cfg.InstallBundle,
		installRoot:    cfg.InstallRoot,
		portableData:   cfg.PortableData,
		bundle:         cfg.Bundle,
		portable:       cfg.Portable,
		settingsReader: cfg.SettingsReader,
		pluginSettings: cfg.PluginSettings,
		startAudit:     cfg.StartAudit,
	}
	if cfg.PluginSettings != nil {
		m.settingsReader = cfg.PluginSettings
	}
	return m
}

// PreviewInstall validates a source plugin directory or bundle without installing.
func (m *PluginManager) PreviewInstall(sourcePath string, policy domainplugin.InstallTrustPolicy) (InstallPreview, error) {
	if m.loadBundle == nil {
		return InstallPreview{}, fmt.Errorf("plugin loader unavailable")
	}
	plugin, err := m.loadBundle(sourcePath)
	if err != nil {
		return InstallPreview{}, fmt.Errorf("load plugin: %w", err)
	}
	trust, err := domainplugin.EvaluateInstallTrust(plugin.Manifest, plugin.ChecksumsDigest, policy)
	if err != nil {
		return InstallPreview{}, err
	}
	return installPreviewFrom(plugin, trust), nil
}

// Install copies the plugin into user storage and registers it.
//
// The grant parameters are the user's install-time answers to the high-impact capabilities the
// preview warned about. Each is refused here rather than persisted: an installed plugin is, by
// construction, one whose declared high-impact capabilities were granted, which is what lets the
// channel resolver treat "installed and declares exec" as consent (ADR-011 D3).
func (m *PluginManager) Install(sourcePath string, policy domainplugin.InstallTrustPolicy, grantMultiSession bool, grantArbitraryNetworkAccess bool, grantExecAccess bool) (domainplugin.InstalledPlugin, error) {
	if m.portable != nil {
		if err := m.portable.RequireWritable(); err != nil {
			return domainplugin.InstalledPlugin{}, err
		}
	}
	if m.installBundle == nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("plugin installer unavailable")
	}
	if m.loadBundle != nil {
		plugin, err := m.loadBundle(sourcePath)
		if err != nil {
			return domainplugin.InstalledPlugin{}, fmt.Errorf("load plugin: %w", err)
		}
		if _, err := domainplugin.EvaluateInstallTrust(plugin.Manifest, plugin.ChecksumsDigest, policy); err != nil {
			return domainplugin.InstalledPlugin{}, err
		}
	}
	installed, err := m.installBundle(sourcePath, m.installRoot)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	// A refused install must leave nothing behind. installBundle has already written the plugin
	// into installRoot by this point, and Discovery.Discover() adopts whatever it finds there on
	// the next start - so returning an error without removing the files turned "the user declined"
	// into "the user declined, and it installed anyway after a restart".
	if err := consentError(installed.Manifest, grantMultiSession, grantArbitraryNetworkAccess, grantExecAccess); err != nil {
		m.discardInstalledFiles(installed)
		return domainplugin.InstalledPlugin{}, err
	}
	if err := m.registry.Register(installed); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if installed.Manifest.RequiresMultiSessionWarning() {
		m.auditStart(installed.Manifest.ID, "install", "allowMultiSession", false)
	}
	if installed.Manifest.RequiresArbitraryNetworkAccess() && grantArbitraryNetworkAccess {
		m.auditStart(installed.Manifest.ID, "install", "allowArbitraryNetwork", false)
	}
	if installed.Manifest.RequiresChannelExecConsent() {
		m.auditStart(installed.Manifest.ID, "install", "channelExec", false)
	}
	return installed, nil
}

func installPreviewFrom(p domainplugin.InstalledPlugin, trust domainplugin.InstallTrustResult) InstallPreview {
	unsigned := trust.UnsignedWarning || trust.UntrustedSignatureWarning
	return InstallPreview{
		ID:                           p.Manifest.ID,
		Name:                         p.Manifest.Name,
		Version:                      p.Manifest.Version,
		Description:                  p.Manifest.Description,
		Signed:                       trust.Signed,
		SignatureVerified:            trust.SignatureVerified,
		ChecksumPresent:              trust.ChecksumPresent,
		RequiresSecretAccess:         p.Manifest.RequiresSecretAccess(),
		RequiresAuthProviderAccess:   p.Manifest.RequiresAuthProviderAccess(),
		RequiresTunnelProviderAccess: p.Manifest.RequiresTunnelProviderAccess(),
		MultiSessionWarning:          p.Manifest.RequiresMultiSessionWarning() || trust.MultiSessionWarning,
		ArbitraryNetworkWarning:      p.Manifest.RequiresArbitraryNetworkWarning() || trust.ArbitraryNetworkWarning,
		ExecAccessWarning:            p.Manifest.RequiresChannelExecConsent(),
		UnsignedWarning:              unsigned,
		UntrustedSignatureWarning:    trust.UntrustedSignatureWarning,
		Permissions:                  p.Manifest.PermissionSummary(),
	}
}

// consentError reports the first install-time consent the caller did not supply.
//
// These three are checked here and nowhere else at install time, which is why the rollback above
// matters: the runtime grant maps gate what an installed plugin may reach, but nothing else gates
// whether it is installed and running at all.
func consentError(manifest domainplugin.Manifest, grantMultiSession, grantArbitraryNetworkAccess, grantExecAccess bool) error {
	switch {
	case manifest.RequiresMultiSessionWarning() && !grantMultiSession:
		return fmt.Errorf("multi-session consent required for this plugin")
	case manifest.RequiresArbitraryNetworkAccess() && !grantArbitraryNetworkAccess:
		return fmt.Errorf("arbitrary network access consent required for this plugin")
	case manifest.RequiresChannelExecConsent() && !grantExecAccess:
		return fmt.Errorf("exec channel consent required for this plugin")
	}
	return nil
}

// discardInstalledFiles removes a plugin tree that was written to disk before an install was
// refused. A failure is logged and not returned: the caller is already returning the refusal, and
// that is the answer the user needs to see.
func (m *PluginManager) discardInstalledFiles(installed domainplugin.InstalledPlugin) {
	if m.portableData == nil || installed.RootDir == "" {
		slog.Error("cannot remove the files of a refused install",
			"plugin", installed.Manifest.ID, "rootDir", installed.RootDir)
		return
	}
	if err := m.portableData.Remove(installed.RootDir); err != nil {
		slog.Error("failed to remove the files of a refused install",
			"plugin", installed.Manifest.ID, "rootDir", installed.RootDir, "error", err)
	}
}
