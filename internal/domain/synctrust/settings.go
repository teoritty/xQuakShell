package synctrust

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"xquakshell/internal/domain"
)

// SettingClass says whether a top-level AppSettings field leaves the device (spec §6.2).
type SettingClass int

const (
	// SettingSynced travels and applies on arrival, last writer wins.
	SettingSynced SettingClass = iota + 1
	// SettingTrust travels, but an incoming value that weakens this device waits for the master
	// password; Classify makes that call, not the settings split.
	SettingTrust
	// SettingDeviceLocal never leaves the device.
	SettingDeviceLocal
)

// settingClasses is keyed by the AppSettings json name. The reflection test in settings_test.go fails
// on any field missing here, so a new setting cannot ship without someone deciding where it goes.
//
// localTerminal and plugins are synced sections that each carry a device-local field: the shell is a
// path-like choice that only makes sense on one machine, and AllowUnsandboxedFallback exists for one
// machine whose confinement fails. Their wire types below simply have no such field, so it can
// neither leave nor arrive.
var settingClasses = map[string]SettingClass{
	"lockout":            SettingTrust,
	"terminal":           SettingSynced,
	"theme":              SettingSynced,
	"language":           SettingSynced,
	"uiScalePercent":     SettingDeviceLocal,
	"ping":               SettingSynced,
	"transfer":           SettingSynced,
	"sessionHotkeys":     SettingSynced,
	"localTerminal":      SettingSynced,
	"terminalTools":      SettingSynced,
	"externalEditorPath": SettingDeviceLocal,
	"auditLog":           SettingDeviceLocal,
	"plugins":            SettingTrust,
	"embed":              SettingSynced,
	"debug":              SettingDeviceLocal,
	"updates":            SettingSynced,
	"reconnect":          SettingSynced,
}

// SettingClassOf reports the class of one AppSettings field by its json name.
func SettingClassOf(jsonName string) (SettingClass, bool) {
	c, ok := settingClasses[jsonName]
	return c, ok
}

// ErrSettings is a settings section that is unknown or does not decode strictly.
var ErrSettings = errors.New("malformed settings section")

type appearanceSection struct {
	Theme    string `json:"theme"`
	Language string `json:"language,omitempty"`
}

type localTerminalSection struct {
	OpenHotkey string `json:"openHotkey,omitempty"`
}

type pluginsSection struct {
	TrustedPublisherKeys          []string        `json:"trustedPublisherKeys,omitempty"`
	RequireSignedPlugins          bool            `json:"requireSignedPlugins,omitempty"`
	SecretAccessGranted           map[string]bool `json:"secretAccessGranted,omitempty"`
	AuthProviderAccessGranted     map[string]bool `json:"authProviderAccessGranted,omitempty"`
	TunnelProviderAccessGranted   map[string]bool `json:"tunnelProviderAccessGranted,omitempty"`
	MultiSessionAccessGranted     map[string]bool `json:"multiSessionAccessGranted,omitempty"`
	ArbitraryNetworkAccessGranted map[string]bool `json:"arbitraryNetworkAccessGranted,omitempty"`
	Disabled                      map[string]bool `json:"disabled,omitempty"`
}

type sectionCodec struct {
	encode func(domain.AppSettings) ([]byte, error)
	apply  func(*domain.AppSettings, []byte) error
}

// codec builds a section from a projection of AppSettings. Applying decodes into a fresh value, not
// over the local one: the incoming section is the sender's whole state for it, and a field the sender
// left at its zero value (and so omitted) must reset here too, or two devices would never converge.
func codec[T any](get func(domain.AppSettings) T, put func(*domain.AppSettings, T)) sectionCodec {
	return sectionCodec{
		encode: func(s domain.AppSettings) ([]byte, error) { return json.Marshal(get(s)) },
		apply: func(s *domain.AppSettings, raw []byte) error {
			var v T
			if err := decodeStrict(raw, &v); err != nil {
				return err
			}
			put(s, v)
			return nil
		},
	}
}

func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %w", ErrSettings, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: data after the section", ErrSettings)
	}
	return nil
}

var sectionCodecs = map[string]sectionCodec{
	"lockout": codec(func(s domain.AppSettings) domain.LockoutSettings { return s.Lockout },
		func(s *domain.AppSettings, v domain.LockoutSettings) { s.Lockout = v }),
	"terminal": codec(func(s domain.AppSettings) domain.TerminalSettings { return s.Terminal },
		func(s *domain.AppSettings, v domain.TerminalSettings) { s.Terminal = v }),
	"appearance": codec(func(s domain.AppSettings) appearanceSection {
		return appearanceSection{Theme: s.Theme, Language: s.Language}
	}, func(s *domain.AppSettings, v appearanceSection) { s.Theme, s.Language = v.Theme, v.Language }),
	"ping": codec(func(s domain.AppSettings) domain.PingSettings { return s.Ping },
		func(s *domain.AppSettings, v domain.PingSettings) { s.Ping = v }),
	"transfer": codec(func(s domain.AppSettings) domain.TransferSettings { return s.Transfer },
		func(s *domain.AppSettings, v domain.TransferSettings) { s.Transfer = v }),
	"sessionHotkeys": codec(func(s domain.AppSettings) domain.SessionHotkeysSettings { return s.SessionHotkeys },
		func(s *domain.AppSettings, v domain.SessionHotkeysSettings) { s.SessionHotkeys = v }),
	"localTerminal": codec(func(s domain.AppSettings) localTerminalSection {
		return localTerminalSection{OpenHotkey: s.LocalTerminal.OpenHotkey}
	}, func(s *domain.AppSettings, v localTerminalSection) { s.LocalTerminal.OpenHotkey = v.OpenHotkey }),
	"terminalTools": codec(func(s domain.AppSettings) domain.TerminalToolsSettings { return s.TerminalTools },
		func(s *domain.AppSettings, v domain.TerminalToolsSettings) { s.TerminalTools = v }),
	"plugins": codec(pluginsOf, applyPlugins),
	"embed": codec(func(s domain.AppSettings) domain.EmbedSettings { return s.Embed },
		func(s *domain.AppSettings, v domain.EmbedSettings) { s.Embed = v }),
	"updates": codec(func(s domain.AppSettings) domain.UpdateSettings { return s.Updates },
		func(s *domain.AppSettings, v domain.UpdateSettings) { s.Updates = v }),
	"reconnect": codec(func(s domain.AppSettings) domain.ReconnectSettings { return s.Reconnect },
		func(s *domain.AppSettings, v domain.ReconnectSettings) { s.Reconnect = v }),
}

func pluginsOf(s domain.AppSettings) pluginsSection {
	p := s.Plugins
	return pluginsSection{
		TrustedPublisherKeys: p.TrustedPublisherKeys, RequireSignedPlugins: p.RequireSignedPlugins,
		SecretAccessGranted: p.SecretAccessGranted, AuthProviderAccessGranted: p.AuthProviderAccessGranted,
		TunnelProviderAccessGranted: p.TunnelProviderAccessGranted, MultiSessionAccessGranted: p.MultiSessionAccessGranted,
		ArbitraryNetworkAccessGranted: p.ArbitraryNetworkAccessGranted, Disabled: p.Disabled,
	}
}

func applyPlugins(s *domain.AppSettings, v pluginsSection) {
	fallback := s.Plugins.AllowUnsandboxedFallback
	s.Plugins = domain.PluginSettings{
		TrustedPublisherKeys: v.TrustedPublisherKeys, RequireSignedPlugins: v.RequireSignedPlugins,
		SecretAccessGranted: v.SecretAccessGranted, AuthProviderAccessGranted: v.AuthProviderAccessGranted,
		TunnelProviderAccessGranted: v.TunnelProviderAccessGranted, MultiSessionAccessGranted: v.MultiSessionAccessGranted,
		ArbitraryNetworkAccessGranted: v.ArbitraryNetworkAccessGranted, Disabled: v.Disabled,
		AllowUnsandboxedFallback: fallback,
	}
}

// SplitSettings serialises every synced section of s, one entry per settings record.
func SplitSettings(s domain.AppSettings) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(sectionCodecs))
	for name, c := range sectionCodecs {
		raw, err := c.encode(s)
		if err != nil {
			return nil, fmt.Errorf("encode settings %s: %w", name, err)
		}
		out[name] = raw
	}
	return out, nil
}

// ApplySection returns local with one incoming section applied and every device-local field kept.
// It applies the data as given: whether a trust-classified section may apply at all is Classify's
// decision, made before this is called. Unknown sections and unknown fields are refused - a section
// is untrusted input from another device.
func ApplySection(local domain.AppSettings, section string, data json.RawMessage) (domain.AppSettings, error) {
	c, ok := sectionCodecs[section]
	if !ok {
		return local, fmt.Errorf("%w: unknown section %q", ErrSettings, section)
	}
	out := local
	if err := c.apply(&out, data); err != nil {
		return local, err
	}
	return out, nil
}
