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
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Version          string         `json:"version"`
	Description      string         `json:"description,omitempty"`
	MinCoreVersion   string         `json:"minCoreVersion,omitempty"`
	Engine           EngineConfig   `json:"engine"`
	Capabilities     CapabilitySet  `json:"capabilities,omitempty"`
	Contributions    Contributions  `json:"contributions,omitempty"`
	ActivationEvents []string       `json:"activationEvents,omitempty"`
	Isolation        IsolationMode  `json:"isolation,omitempty"`
	Signature        string         `json:"signature,omitempty"`
}

// EngineConfig locates the plugin binary.
type EngineConfig struct {
	Type  string   `json:"type"`
	Entry string   `json:"entry"`
	Args  []string `json:"args,omitempty"`
}

// CapabilitySet declares permissions requested at install (ADR-002).
type CapabilitySet struct {
	Network  *NetworkCaps  `json:"network,omitempty"`
	FS       *FSCaps       `json:"filesystem,omitempty"`
	Events   *EventCaps    `json:"events,omitempty"`
	Vault    *VaultCaps    `json:"vault,omitempty"`
	Session  *SessionCaps  `json:"session,omitempty"`
}

// NetworkCaps controls outbound connectivity.
type NetworkCaps struct {
	Outbound []string `json:"outbound,omitempty"`
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
	ConnectProtocols  []string `json:"connectProtocols,omitempty"`
	Terminal          bool     `json:"terminal,omitempty"`
	RemoteFS          bool     `json:"remoteFs,omitempty"`
	AllowMultiSession bool     `json:"allowMultiSession,omitempty"`
}

// Contributions holds declarative UI extension points.
type Contributions struct {
	Commands            []CommandContribution            `json:"commands,omitempty"`
	Menus               []MenuContribution               `json:"menus,omitempty"`
	ConnectionProtocols []ConnectionProtocolContribution `json:"connectionProtocols,omitempty"`
	Views               []ViewContribution               `json:"views,omitempty"`
	StatusBar           []StatusBarContribution          `json:"statusBar,omitempty"`
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
	ID          string `json:"id"`
	Label       string `json:"label"`
	DefaultPort int    `json:"defaultPort,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// ViewContribution registers a declarative UI panel (Phase 5).
type ViewContribution struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`
	Entry    string `json:"entry,omitempty"`
}

// StatusBarContribution registers a status bar item (Phase 5).
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
}

// InstallSource indicates where the plugin was loaded from.
type InstallSource string

const (
	SourceBundled    InstallSource = "bundled"
	SourceUser       InstallSource = "user"
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
	if err := ValidateMinCoreVersion(m.MinCoreVersion); err != nil {
		return err
	}
	if err := m.CompatibleWithCore(HostCoreVersion); err != nil {
		return err
	}
	return m.ValidateCapabilities()
}

// EntryPath resolves the plugin binary path relative to RootDir.
func (p InstalledPlugin) EntryPath() string {
	return filepath.Join(p.RootDir, p.Manifest.Engine.Entry)
}
