package plugin

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// EngineGoBinary identifies a native Go plugin executable.
	EngineGoBinary = "go-binary"
	// DefaultIsolation is one process per plugin ID (ADR-003).
	DefaultIsolation = IsolationPerPlugin
)

// IsolationMode controls how plugin processes are spawned (ADR-003).
type IsolationMode string

const (
	IsolationPerPlugin  IsolationMode = "per-plugin"
	IsolationPerSession IsolationMode = "per-session"
)

// Manifest describes a plugin package (plugin.json).
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	// MinCoreVersion is unsupported; kept only so resolvePluginAPI can reject
	// manifests that still declare it with a clear error.
	MinCoreVersion   string          `json:"minCoreVersion,omitempty"`
	Requires         *RequirementSet `json:"requires,omitempty"`
	Engine           EngineConfig    `json:"engine"`
	Capabilities     CapabilitySet   `json:"capabilities,omitempty"`
	Contributions    Contributions   `json:"contributions,omitempty"`
	ActivationEvents []string        `json:"activationEvents,omitempty"`
	Isolation        IsolationMode   `json:"isolation,omitempty"`
	Signature        string          `json:"signature,omitempty"`
}

// EngineConfig locates the plugin binary.
type EngineConfig struct {
	Type  string   `json:"type"`
	Entry string   `json:"entry"`
	Args  []string `json:"args,omitempty"`
}

// CapabilitySet declares permissions requested at install (ADR-002).
type CapabilitySet struct {
	Network *NetworkCaps `json:"network,omitempty"`
	FS      *FSCaps      `json:"filesystem,omitempty"`
	Events  *EventCaps   `json:"events,omitempty"`
	Vault   *VaultCaps   `json:"vault,omitempty"`
	Session *SessionCaps `json:"session,omitempty"`
	Auth    *AuthCaps    `json:"auth,omitempty"`
	Tunnel  *TunnelCaps  `json:"tunnel,omitempty"`
	Channel *ChannelCaps `json:"channel,omitempty"`
	// Discovery declares the ability to draw resource subtrees inside connections (ADR-014).
	Discovery *DiscoveryCaps `json:"discovery,omitempty"`
	// UI declares where a plugin may draw its own tabs, dialogs and node details (ADR-015).
	UI *UICaps `json:"ui,omitempty"`
}

// NetworkCaps controls outbound connectivity.
type NetworkCaps struct {
	Outbound               []string `json:"outbound,omitempty"`
	AllowArbitraryOutbound bool     `json:"allowArbitraryOutbound,omitempty"`
	AllowPrivateNetworks   bool     `json:"allowPrivateNetworks,omitempty"`
}

// FSCaps controls sandboxed file access.
type FSCaps struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

// EventCaps controls event bus access.
type EventCaps struct {
	Subscribe []string `json:"subscribe,omitempty"`
	Publish   []string `json:"publish,omitempty"`
}

// VaultCaps controls vault field access (ADR-002).
type VaultCaps struct {
	ReadConnectionFields []string `json:"readConnectionFields,omitempty"`
	GetSecret            []string `json:"getSecret,omitempty"`
}

// SessionCaps declares session-related permissions.
type SessionCaps struct {
	ConnectProtocols       []string `json:"connectProtocols,omitempty"`
	Terminal               bool     `json:"terminal,omitempty"`
	Embed                  bool     `json:"embed,omitempty"`
	LocalEmbedServer       bool     `json:"localEmbedServer,omitempty"`
	RemoteFS               bool     `json:"remoteFs,omitempty"`
	AllowMultiSession      bool     `json:"allowMultiSession,omitempty"`
	MaxTunnelBandwidthKbps int      `json:"maxTunnelBandwidthKbps,omitempty"`
}

// AuthCaps declares that a plugin can act as an SSH auth provider.
type AuthCaps struct {
	Provider bool     `json:"provider,omitempty"`
	Methods  []string `json:"methods,omitempty"`
}

// TunnelCaps declares that a plugin can decide routing for dynamic forward rules.
type TunnelCaps struct {
	Provider              bool `json:"provider,omitempty"`
	MaxConcurrentChannels int  `json:"maxConcurrentChannels,omitempty"`
}

// Contributions holds declarative UI extension points.
type Contributions struct {
	Commands            []CommandContribution            `json:"commands,omitempty"`
	Menus               []MenuContribution               `json:"menus,omitempty"`
	ConnectionProtocols []ConnectionProtocolContribution `json:"connectionProtocols,omitempty"`
	Views               []ViewContribution               `json:"views,omitempty"`
	StatusBar           []StatusBarContribution          `json:"statusBar,omitempty"`
	AuthMethods         []AuthMethodContribution         `json:"authMethods,omitempty"`
	TunnelProviders     []TunnelProviderContribution     `json:"tunnelProviders,omitempty"`
	// DiscoveryIcons registers the icon assets a discovery plugin's nodes may reference by ID
	// (ADR-014). Meaningless without capabilities.discovery — see validateDiscoveryCaps.
	DiscoveryIcons []DiscoveryIconContribution `json:"discoveryIcons,omitempty"`
}

// CommandContribution registers a command palette entry.
type CommandContribution struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`
}

// MenuContribution registers a menu location.
type MenuContribution struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Location string   `json:"location"`
	Items    []string `json:"items"`
}

// ConnectionProtocolContribution registers a connection protocol.
type ConnectionProtocolContribution struct {
	ID          string       `json:"id"`
	Label       string       `json:"label"`
	DefaultPort int          `json:"defaultPort,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	EmbedEntry  string       `json:"embedEntry,omitempty"`
	Fields      []FieldGroup `json:"fields,omitempty"`
}

// AuthMethodContribution registers one auth method the plugin offers.
type AuthMethodContribution struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Kind   string       `json:"kind"`
	Fields []FieldGroup `json:"fields,omitempty"`
}

// TunnelProviderContribution registers one dynamic-forward routing strategy.
type TunnelProviderContribution struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

const defaultEmbedEntry = "ui/embed.html"

// ViewContribution registers a declarative UI panel.
type ViewContribution struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`
	Entry    string `json:"entry,omitempty"`
}

// StatusBarContribution registers a status bar item.
type StatusBarContribution struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Tooltip  string `json:"tooltip,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// HasViews reports whether the plugin contributes UI views.
func (m *Manifest) HasViews() bool {
	return len(m.Contributions.Views) > 0
}

// InstalledPlugin is a discovered manifest with its on-disk location.
type InstalledPlugin struct {
	Manifest Manifest
	RootDir  string
	Source   InstallSource
	// InstallMeta records provenance for user-installed plugins (e.g. GitHub release tag).
	InstallMeta *PluginInstallMeta
	// ChecksumsDigest is the hex-encoded SHA-256 digest of the bundle's SHA256SUMS file,
	// computed at load time immediately after checksum validation (before any temp-dir cleanup).
	// Empty when the bundle has no SHA256SUMS. Used to bind manifest signature verification
	// to bundle contents.
	ChecksumsDigest string
}

// InstallSource indicates where the plugin was loaded from.
type InstallSource string

const (
	SourceBundled InstallSource = "bundled"
	SourceUser    InstallSource = "user"
)

// EffectiveIsolation returns the isolation mode from manifest or default.
func (m *Manifest) EffectiveIsolation() IsolationMode {
	if m.Isolation == "" {
		return DefaultIsolation
	}
	return m.Isolation
}

// RequiresSecretAccess reports whether the plugin declared vault.getSecret (ADR-002).
func (m *Manifest) RequiresSecretAccess() bool {
	return m.Capabilities.Vault != nil && len(m.Capabilities.Vault.GetSecret) > 0
}

// RequiresAuthProviderAccess reports whether the plugin declared auth.provider.
func (m *Manifest) RequiresAuthProviderAccess() bool {
	return m.Capabilities.Auth != nil && m.Capabilities.Auth.Provider
}

// RequiresTunnelProviderAccess reports whether the plugin declared tunnel.provider.
func (m *Manifest) RequiresTunnelProviderAccess() bool {
	return m.Capabilities.Tunnel != nil && m.Capabilities.Tunnel.Provider
}

// Validate checks required manifest fields.
func (m *Manifest) Validate() error {
	if err := ValidateID(m.ID); err != nil {
		return err
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("%w: version is required", ErrInvalidManifest)
	}
	if m.Engine.Type != EngineGoBinary {
		return fmt.Errorf("%w: unsupported engine type %q", ErrInvalidManifest, m.Engine.Type)
	}
	if strings.TrimSpace(m.Engine.Entry) == "" {
		return fmt.Errorf("%w: engine.entry is required", ErrInvalidManifest)
	}
	if err := ValidateBundleRelativePath(m.Engine.Entry); err != nil {
		return err
	}
	if len(m.Engine.Args) > 0 {
		return fmt.Errorf("%w: engine.args is not supported in v1", ErrInvalidManifest)
	}
	if m.Isolation != "" && m.Isolation != IsolationPerPlugin && m.Isolation != IsolationPerSession {
		return fmt.Errorf("%w: invalid isolation %q", ErrInvalidManifest, m.Isolation)
	}
	if err := m.ValidateCapabilitiesAndFields(); err != nil {
		return err
	}
	// Structural version contract (ADR-012): the requires{} block must be well-formed and only
	// reference granted capabilities. HOST compatibility (whether this build satisfies the plugin)
	// is deliberately NOT checked here — Validate is used to parse and display manifests, including
	// ones this host cannot run. Compatibility is gated separately via CheckHostCompatibility at
	// discovery/install and via Negotiate at spawn.
	return m.ValidateRequirements()
}

// CheckHostCompatibility reports whether this host can satisfy the plugin's declared API and
// capability requirements (ADR-012). It is the gating check for discovery and install — distinct
// from Validate, which only checks that the manifest is well-formed. Returns a structured
// IncompatibilityReport / ErrIncompatibleAPI when the plugin cannot run on this build.
func (m *Manifest) CheckHostCompatibility() error {
	_, _, err := Negotiate(m, HostRegistry())
	return err
}

// ValidateCapabilitiesAndFields runs capability and contribution field validation.
func (m *Manifest) ValidateCapabilitiesAndFields() error {
	if err := m.ValidateCapabilities(); err != nil {
		return err
	}
	return ValidateManifestFields(m)
}

// EntryPath resolves the plugin binary path relative to RootDir.
func (p InstalledPlugin) EntryPath() string {
	return filepath.Join(p.RootDir, p.Manifest.Engine.Entry)
}
