package plugin

import (
	"fmt"
	"strings"
)

const placeholderPluginData = "${pluginData}"

var allowedCoreEventChannels = map[string]struct{}{
	"core.session.opened":       {},
	"core.session.closed":       {},
	"core.session.stateChanged": {},
}

// ValidateCapabilities rejects unsafe capability declarations (string rules only).
// On-disk FS root checks run via bundle.ValidateCapabilitiesForInstall in infra.
func (m *Manifest) ValidateCapabilities() error {
	if m.Capabilities.Network != nil {
		for _, pattern := range m.Capabilities.Network.Outbound {
			if err := validateNetworkPattern(pattern); err != nil {
				return err
			}
		}
	}
	if m.Capabilities.FS != nil {
		patterns := append(append([]string{}, m.Capabilities.FS.Read...), m.Capabilities.FS.Write...)
		for _, p := range patterns {
			p = strings.TrimSpace(p)
			if p == "*" {
				return ErrInvalidManifest
			}
			if !strings.HasPrefix(p, placeholderPluginData) {
				return ErrInvalidManifest
			}
		}
	}
	if m.Capabilities.Events != nil {
		for _, ch := range m.Capabilities.Events.Subscribe {
			if err := validateSubscribeChannel(m.ID, ch); err != nil {
				return err
			}
		}
		for _, ch := range m.Capabilities.Events.Publish {
			if err := validatePublishChannel(m.ID, ch); err != nil {
				return err
			}
		}
	}
	if m.Capabilities.Vault != nil {
		for _, field := range m.Capabilities.Vault.GetSecret {
			if !allowedSecretField(field) {
				return ErrInvalidManifest
			}
		}
	}
	for _, ev := range m.ActivationEvents {
		if strings.TrimSpace(ev) == "*" {
			return ErrInvalidManifest
		}
	}
	if err := m.validateConnectionProtocolCaps(); err != nil {
		return err
	}
	if err := m.validateSessionCaps(); err != nil {
		return err
	}
	if err := m.validateViewEntries(); err != nil {
		return err
	}
	return nil
}

func (m *Manifest) validateSessionCaps() error {
	if m.Capabilities.Session == nil {
		return nil
	}
	if m.Capabilities.Session.AllowMultiSession {
		if m.Capabilities.Session.Terminal {
			return fmt.Errorf("%w: terminal plugins may not use allowMultiSession", ErrInvalidManifest)
		}
		if m.EffectiveIsolation() != IsolationPerPlugin {
			return fmt.Errorf("%w: allowMultiSession requires per-plugin isolation", ErrInvalidManifest)
		}
	}
	if m.Capabilities.Session.Terminal && m.EffectiveIsolation() != IsolationPerSession {
		return fmt.Errorf("%w: terminal requires isolation per-session", ErrInvalidManifest)
	}
	return nil
}

func (m *Manifest) validateViewEntries() error {
	if m.HasViews() && m.EffectiveIsolation() != IsolationPerPlugin {
		return fmt.Errorf("%w: webview contributions require per-plugin isolation", ErrInvalidManifest)
	}
	for _, v := range m.Contributions.Views {
		entry := strings.TrimSpace(v.Entry)
		if entry == "" {
			entry = "ui/index.html"
		}
		if err := ValidateViewAssetEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

// RequiresMultiSessionWarning reports whether install should warn about shared-process terminal access.
func (m *Manifest) RequiresMultiSessionWarning() bool {
	if m.Capabilities.Session == nil {
		return false
	}
	return m.Capabilities.Session.AllowMultiSession && !m.Capabilities.Session.Terminal
}

// AllowsConnectProtocol reports whether the manifest permits handling the given connection protocol.
func (m *Manifest) AllowsConnectProtocol(protocol string) bool {
	protocol = strings.TrimSpace(protocol)
	if protocol == "" || m.Capabilities.Session == nil {
		return false
	}
	for _, allowed := range m.Capabilities.Session.ConnectProtocols {
		if strings.TrimSpace(allowed) == protocol {
			return true
		}
	}
	return false
}

func (m *Manifest) validateConnectionProtocolCaps() error {
	if len(m.Contributions.ConnectionProtocols) == 0 {
		return nil
	}
	if m.Capabilities.Session == nil || len(m.Capabilities.Session.ConnectProtocols) == 0 {
		return fmt.Errorf("%w: connectionProtocols require capabilities.session.connectProtocols", ErrInvalidManifest)
	}
	allowed := make(map[string]struct{}, len(m.Capabilities.Session.ConnectProtocols))
	for _, p := range m.Capabilities.Session.ConnectProtocols {
		p = strings.TrimSpace(p)
		if p == "" {
			return ErrInvalidManifest
		}
		allowed[p] = struct{}{}
	}
	for _, cp := range m.Contributions.ConnectionProtocols {
		id := strings.TrimSpace(cp.ID)
		if id == "" {
			return ErrInvalidManifest
		}
		if _, ok := allowed[id]; !ok {
			return fmt.Errorf("%w: connection protocol %q not listed in capabilities.session.connectProtocols", ErrInvalidManifest, id)
		}
	}
	return nil
}

func validateSubscribeChannel(pluginID, channel string) error {
	channel = strings.TrimSpace(channel)
	if channel == "" || channel == "*" || channel == "core.*" {
		return ErrInvalidManifest
	}
	if strings.HasSuffix(channel, "*") {
		prefix := strings.TrimSuffix(channel, "*")
		if prefix == "core." {
			return ErrInvalidManifest
		}
	}
	if strings.HasPrefix(channel, "core.") {
		if _, ok := allowedCoreEventChannels[channel]; !ok {
			if !strings.HasSuffix(channel, ".*") {
				return ErrInvalidManifest
			}
			switch channel {
			case "core.session.*":
				return nil
			default:
				return ErrInvalidManifest
			}
		}
	}
	return ValidatePluginEventPattern(pluginID, channel)
}

func validatePublishChannel(pluginID, channel string) error {
	channel = strings.TrimSpace(channel)
	if channel == "" || channel == "*" {
		return ErrInvalidManifest
	}
	if strings.HasPrefix(channel, "core.") {
		return fmt.Errorf("%w: plugins may not publish to core channels", ErrInvalidManifest)
	}
	if !strings.HasPrefix(channel, "plugin.") {
		return fmt.Errorf("%w: publish channel must use plugin namespace", ErrInvalidManifest)
	}
	return ValidatePluginEventPattern(pluginID, channel)
}

func validateNetworkPattern(pattern string) error {
	_, err := ParseNetworkPattern(pattern)
	return err
}

func allowedSecretField(field string) bool {
	switch field {
	case "password", "privateKey", "passphrase":
		return true
	default:
		return false
	}
}

// PermissionSummary returns human-readable install-time permission lines.
func (m *Manifest) PermissionSummary() []string {
	var lines []string
	if m.Capabilities.FS != nil {
		if len(m.Capabilities.FS.Read) > 0 {
			lines = append(lines, "Read files in declared sandbox paths")
		}
		if len(m.Capabilities.FS.Write) > 0 {
			lines = append(lines, "Write files in declared sandbox paths")
		}
	}
	if m.Capabilities.Network != nil && len(m.Capabilities.Network.Outbound) > 0 {
		lines = append(lines, "Outbound network: "+strings.Join(m.Capabilities.Network.Outbound, ", "))
	}
	if m.Capabilities.Vault != nil && len(m.Capabilities.Vault.ReadConnectionFields) > 0 {
		lines = append(lines, "Read connection metadata (no secrets by default)")
	}
	if m.RequiresSecretAccess() {
		lines = append(lines, "Access connection secrets: "+strings.Join(m.Capabilities.Vault.GetSecret, ", "))
	}
	if m.RequiresMultiSessionWarning() {
		lines = append(lines, "Shared process may access multiple sessions (allowMultiSession)")
	}
	if len(lines) == 0 {
		lines = append(lines, "No elevated permissions requested")
	}
	return lines
}
