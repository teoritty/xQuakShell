package domain

// TerminalSettings configures the embedded terminal appearance.
type TerminalSettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	FontColor  string `json:"fontColor"`
}

// DefaultTerminalSettings supplies what a profile starts with before the user
// has saved anything. The font stack is ordered by what a Windows install is
// likeliest to already have, ending in the generic monospace family so the
// terminal still renders on a machine with none of the named fonts.
func DefaultTerminalSettings() TerminalSettings {
	return TerminalSettings{
		FontFamily: "Cascadia Code, Consolas, Courier New, monospace",
		FontSize:   14,
		FontColor:  "#cccccc",
	}
}

// PingMode controls when ping runs: "interval" = periodic, "on_change" = only when connection settings change.
const (
	PingModeInterval = "interval"
	PingModeOnChange = "on_change"
)

// PingSettings configures automatic host reachability checks.
type PingSettings struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	IntervalSeconds int    `json:"intervalSeconds"`
	MaxConcurrent   int    `json:"maxConcurrent"`
}

func (p PingSettings) EffectiveIntervalSeconds() int {
	if p.IntervalSeconds > 0 {
		return p.IntervalSeconds
	}
	return 5
}

func (p PingSettings) EffectiveMaxConcurrent() int {
	if p.MaxConcurrent > 0 {
		return p.MaxConcurrent
	}
	return 16
}

func DefaultPingSettings() PingSettings {
	return PingSettings{
		Enabled:         true,
		Mode:            PingModeInterval,
		IntervalSeconds: 5,
		MaxConcurrent:   16,
	}
}

// TransferSettings configures file transfer behavior.
//
// DefaultUploadExistsAction and DefaultDownloadExistsAction hold the persisted
// "file already exists" action (FileZilla-style), as ConflictAction wire names
// (see domain.ParseConflictAction). Empty or "ask" means prompt with the
// conflict dialog; any other value is applied without prompting.
type TransferSettings struct {
	SpeedLimitKbps              int    `json:"speedLimitKbps"`
	ConnectionTimeoutSec        int    `json:"connectionTimeoutSec"`
	MaxConcurrent               int    `json:"maxConcurrent"`
	DefaultUploadExistsAction   string `json:"defaultUploadExistsAction,omitempty"`
	DefaultDownloadExistsAction string `json:"defaultDownloadExistsAction,omitempty"`
}

func DefaultTransferSettings() TransferSettings {
	return TransferSettings{
		SpeedLimitKbps:       0,
		ConnectionTimeoutSec: 15,
		MaxConcurrent:        4,
	}
}

type SessionHotkeysSettings struct {
	Create string `json:"create"`
	Next   string `json:"next"`
	Prev   string `json:"prev"`
	Close  string `json:"close"`
}

func DefaultSessionHotkeysSettings() SessionHotkeysSettings {
	return SessionHotkeysSettings{
		Create: "Ctrl+Shift+N",
		Next:   "Ctrl+Tab",
		Prev:   "Ctrl+Shift+Tab",
		Close:  "Ctrl+Shift+Q",
	}
}

// LocalTerminalSettings holds what the user chose about local shells.
//
// The hotkey lives here rather than beside the session hotkeys because a local terminal is not a
// session - it has no connection, no vault binding and no host key - and putting it in
// SessionHotkeysSettings would be the first step towards code that treats it as one.
type LocalTerminalSettings struct {
	// ShellID names a shell from the closed set the catalog knows, never a filesystem path. An
	// empty value means "whatever this platform's default is", resolved when a terminal opens
	// rather than written here, so a fresh vault never stores a choice the user did not make.
	ShellID string `json:"shellId,omitempty"`
	// OpenHotkey opens a new local terminal.
	OpenHotkey string `json:"openHotkey,omitempty"`
}

// DefaultLocalTerminalSettings supplies the platform default shell and the conventional binding.
func DefaultLocalTerminalSettings() LocalTerminalSettings {
	return LocalTerminalSettings{OpenHotkey: "Ctrl+Shift+T"}
}

type EmbedSettings struct {
	SuspendTcpWhenInactive bool `json:"suspendTcpWhenInactive,omitempty"`
}

type DebugSettings struct {
	LogWindowEnabled bool `json:"logWindowEnabled,omitempty"`
	// LogLevel is the minimum level published to the debug log
	// (debug|info|warn|error). Empty means the host default, which is deliberately
	// quiet: the level decides the cost of every log statement on a per-frame data
	// path, and this field is `omitempty`, so an empty value is what every install
	// that never opened Developer settings resolves through.
	LogLevel string `json:"logLevel,omitempty"`
}

func DefaultDebugSettings() DebugSettings {
	return DebugSettings{}
}

type AppSettings struct {
	Lockout            LockoutSettings        `json:"lockout"`
	Terminal           TerminalSettings       `json:"terminal"`
	Theme              string                 `json:"theme"`
	Language           string                 `json:"language,omitempty"`
	UIScalePercent     int                    `json:"uiScalePercent"`
	Ping               PingSettings           `json:"ping"`
	Transfer           TransferSettings       `json:"transfer"`
	SessionHotkeys     SessionHotkeysSettings `json:"sessionHotkeys"`
	LocalTerminal      LocalTerminalSettings  `json:"localTerminal"`
	ExternalEditorPath string                 `json:"externalEditorPath,omitempty"`
	AuditLog           AuditLogSettings       `json:"auditLog"`
	Plugins            PluginSettings         `json:"plugins"`
	Embed              EmbedSettings          `json:"embed"`
	Debug              DebugSettings          `json:"debug"`
	Updates            UpdateSettings         `json:"updates"`
}

type PluginSettings struct {
	TrustedPublisherKeys          []string        `json:"trustedPublisherKeys,omitempty"`
	RequireSignedPlugins          bool            `json:"requireSignedPlugins,omitempty"`
	SecretAccessGranted           map[string]bool `json:"secretAccessGranted,omitempty"`
	AuthProviderAccessGranted     map[string]bool `json:"authProviderAccessGranted,omitempty"`
	TunnelProviderAccessGranted   map[string]bool `json:"tunnelProviderAccessGranted,omitempty"`
	MultiSessionAccessGranted     map[string]bool `json:"multiSessionAccessGranted,omitempty"`
	ArbitraryNetworkAccessGranted map[string]bool `json:"arbitraryNetworkAccessGranted,omitempty"`
	Disabled                      map[string]bool `json:"disabled,omitempty"`

	// PluginGrants records what each plugin was actually consented to, keyed by the permissions
	// rather than by a bare yes. The five maps above answer "was this id granted X"; they cannot
	// answer "did the bundle behind that id start asking for more", which is the question ADR-022
	// needs and the reason this field exists.
	PluginGrants []PluginGrant `json:"pluginGrants,omitempty"`

	// ScopeRoots marks the folders that expose their contents to a plugin (ADR-022). The core
	// creates one per plugin that declares it needs a scope; a plugin never creates one, or it
	// could draw a second folder resembling the first and collect what was dropped into it.
	ScopeRoots []ScopeRoot `json:"scopeRoots,omitempty"`

	// AllowUnsandboxedFallback lets a plugin start unconfined on a platform that CAN confine it and
	// failed to.
	//
	// It exists because the alternative to refusing is worse: a silent downgrade means the sandbox
	// stops working for a fraction of users and nobody finds out. Refusing is loud, and this is the
	// escape hatch for someone whose machine the confinement will not work on — a policy-locked
	// registry, a filesystem that will not take the ACL — who would otherwise have no way to run a
	// plugin at all.
	//
	// It does NOT cover the platform that cannot confine at all. macOS and a kernel without
	// Landlock start plugins unconfined regardless; that is not a failure and needs no opt-in.
	//
	// Default false, and false is the only safe default: a setting that ships enabled is a sandbox
	// that ships optional. It is a security setting (CLAUDE.md §2.2) — it changes only through
	// saveSettings, enabling it is audit-logged, and no plugin-facing RPC may read or write it.
	AllowUnsandboxedFallback bool `json:"allowUnsandboxedFallback,omitempty"`
}

// DefaultPluginSettings is the plugin policy a vault starts life with.
//
// RequireSignedPlugins is on. A plugin is arbitrary code running on the user's machine, and the
// default decides it for everyone who never opens the settings dialog - which is most people. It
// shipped off because the field is a bool with `omitempty`, so false was simply the zero value
// nobody chose.
//
// This only reaches a NEW vault. An existing one carries its own stored value and keeps it: false
// there may well have been a deliberate choice, and silently overriding a security setting a user
// picked is its own kind of wrong. What protects those installations is that the checksum on a
// release asset is now mandatory regardless of this flag (ErrChecksumUnavailable), and that
// turning this off now costs the master password (PluginTrustWeakened).
func DefaultPluginSettings() PluginSettings {
	return PluginSettings{RequireSignedPlugins: true}
}

// RevokePluginGrants removes every capability grant and the disabled marker recorded for a plugin.
//
// A grant is consent given to a particular plugin. When that plugin is gone, the consent has no
// subject left, and leaving it behind means the next thing installed under the same id inherits a
// decision the user made about something else. plugin.id is chosen by the plugin author and
// verified against nothing, so "the same id" is not a coincidence an attacker has to wait for.
//
// It returns the names of the grants that were actually revoked, so the caller can log what the
// user is losing rather than announcing a revocation that removed nothing.
func (p *PluginSettings) RevokePluginGrants(pluginID string) []string {
	if p == nil || pluginID == "" {
		return nil
	}
	maps := []struct {
		name  string
		grant map[string]bool
	}{
		{"secret", p.SecretAccessGranted},
		{"authProvider", p.AuthProviderAccessGranted},
		{"tunnelProvider", p.TunnelProviderAccessGranted},
		{"multiSession", p.MultiSessionAccessGranted},
		{"arbitraryNetwork", p.ArbitraryNetworkAccessGranted},
	}
	var revoked []string
	for _, m := range maps {
		if m.grant == nil {
			continue
		}
		if m.grant[pluginID] {
			revoked = append(revoked, m.name)
		}
		delete(m.grant, pluginID)
	}
	if p.revokeGrant(pluginID) {
		revoked = append(revoked, "permissions")
	}
	// The disabled marker is not a grant, but it is keyed the same way and is equally stale once
	// the plugin is gone: leaving it would silently disable a different plugin installed later
	// under that id.
	delete(p.Disabled, pluginID)
	return revoked
}
