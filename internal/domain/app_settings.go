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

type EmbedSettings struct {
	SuspendTcpWhenInactive bool `json:"suspendTcpWhenInactive,omitempty"`
}

type DebugSettings struct {
	LogWindowEnabled bool `json:"logWindowEnabled,omitempty"`
	// LogLevel is the minimum level published to the debug log
	// (debug|info|warn|error). Empty means debug (most verbose).
	LogLevel string `json:"logLevel,omitempty"`
}

func DefaultDebugSettings() DebugSettings {
	return DebugSettings{}
}

type AppSettings struct {
	Lockout            LockoutSettings        `json:"lockout"`
	Terminal           TerminalSettings       `json:"terminal"`
	Theme              string                 `json:"theme"`
	UIScalePercent     int                    `json:"uiScalePercent"`
	Ping               PingSettings           `json:"ping"`
	Transfer           TransferSettings       `json:"transfer"`
	SessionHotkeys     SessionHotkeysSettings `json:"sessionHotkeys"`
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

func DefaultPluginSettings() PluginSettings {
	return PluginSettings{}
}
