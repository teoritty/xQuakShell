package domain

// TerminalSettings configures the embedded terminal appearance.
type TerminalSettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	FontColor  string `json:"fontColor"`
}

// DefaultTerminalSettings returns sensible terminal defaults.
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
	IntervalMinutes int    `json:"intervalMinutes"`
}

// EffectiveIntervalSeconds returns the interval in seconds, migrating from legacy intervalMinutes if needed.
func (p PingSettings) EffectiveIntervalSeconds() int {
	if p.IntervalSeconds > 0 {
		return p.IntervalSeconds
	}
	if p.IntervalMinutes > 0 {
		return p.IntervalMinutes * 60
	}
	return 5
}

// DefaultPingSettings returns reasonable defaults for ping monitoring.
func DefaultPingSettings() PingSettings {
	return PingSettings{
		Enabled:         true,
		Mode:            PingModeInterval,
		IntervalSeconds: 5,
	}
}

// TransferSettings configures file transfer behavior.
type TransferSettings struct {
	SpeedLimitKbps       int `json:"speedLimitKbps"`
	ConnectionTimeoutSec int `json:"connectionTimeoutSec"`
	MaxConcurrent        int `json:"maxConcurrent"`
}

// DefaultTransferSettings returns reasonable defaults.
func DefaultTransferSettings() TransferSettings {
	return TransferSettings{
		SpeedLimitKbps:       0,
		ConnectionTimeoutSec: 15,
		MaxConcurrent:        4,
	}
}

// SessionHotkeysSettings configures keyboard shortcuts for session management.
type SessionHotkeysSettings struct {
	Create string `json:"create"`
	Next   string `json:"next"`
	Prev   string `json:"prev"`
	Close  string `json:"close"`
}

// DefaultSessionHotkeysSettings returns default shortcuts for session management.
func DefaultSessionHotkeysSettings() SessionHotkeysSettings {
	return SessionHotkeysSettings{
		Create: "Ctrl+Shift+N",
		Next:   "Ctrl+Tab",
		Prev:   "Ctrl+Shift+Tab",
		Close:  "Ctrl+Shift+Q",
	}
}

// AppSettings stores user-configurable application settings inside the vault.
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
}

// PluginSettings configures plugin trust and install policy.
type PluginSettings struct {
	TrustedPublisherKeys      []string        `json:"trustedPublisherKeys,omitempty"`
	RequireSignedPlugins      bool            `json:"requireSignedPlugins,omitempty"`
	SecretAccessGranted       map[string]bool `json:"secretAccessGranted,omitempty"`
	MultiSessionAccessGranted map[string]bool `json:"multiSessionAccessGranted,omitempty"`
	Disabled                  map[string]bool `json:"disabled,omitempty"`
}

// DefaultPluginSettings returns default plugin install policy.
func DefaultPluginSettings() PluginSettings {
	return PluginSettings{}
}
