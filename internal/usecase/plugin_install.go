package usecase

import (
	"fmt"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// InstallPreview describes a plugin before installation.
type InstallPreview struct {
	ID                        string
	Name                      string
	Version                   string
	Description               string
	Signed                    bool
	SignatureVerified         bool
	ChecksumPresent           bool
	RequiresSecretAccess      bool
	MultiSessionWarning       bool
	UnsignedWarning           bool
	UntrustedSignatureWarning bool
	Permissions               []string
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
	trust, err := domainplugin.EvaluateInstallTrust(plugin.Manifest, policy)
	if err != nil {
		return InstallPreview{}, err
	}
	return installPreviewFrom(m, plugin, trust), nil
}

// Install copies the plugin into user storage and registers it.
func (m *PluginManager) Install(sourcePath string, policy domainplugin.InstallTrustPolicy, grantMultiSession bool) (domainplugin.InstalledPlugin, error) {
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
		if _, err := domainplugin.EvaluateInstallTrust(plugin.Manifest, policy); err != nil {
			return domainplugin.InstalledPlugin{}, err
		}
	}
	installed, err := m.installBundle(sourcePath, m.installRoot)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if installed.Manifest.RequiresMultiSessionWarning() && !grantMultiSession {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("multi-session consent required for this plugin")
	}
	if err := m.registry.Register(installed); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if installed.Manifest.RequiresMultiSessionWarning() {
		m.auditStart(installed.Manifest.ID, "install", "allowMultiSession", false)
	}
	return installed, nil
}

func installPreviewFrom(m *PluginManager, p domainplugin.InstalledPlugin, trust domainplugin.InstallTrustResult) InstallPreview {
	unsigned := trust.UnsignedWarning || trust.UntrustedSignatureWarning
	checksum := false
	if m != nil && m.bundle != nil {
		checksum = m.bundle.HasChecksums(p.RootDir)
	}
	return InstallPreview{
		ID:                        p.Manifest.ID,
		Name:                      p.Manifest.Name,
		Version:                   p.Manifest.Version,
		Description:               p.Manifest.Description,
		Signed:                    trust.Signed,
		SignatureVerified:         trust.SignatureVerified,
		ChecksumPresent:           checksum,
		RequiresSecretAccess:      p.Manifest.RequiresSecretAccess(),
		MultiSessionWarning:       p.Manifest.RequiresMultiSessionWarning() || trust.MultiSessionWarning,
		UnsignedWarning:           unsigned,
		UntrustedSignatureWarning: trust.UntrustedSignatureWarning,
		Permissions:               p.Manifest.PermissionSummary(),
	}
}
