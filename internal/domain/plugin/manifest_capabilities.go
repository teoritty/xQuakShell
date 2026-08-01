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
		if err := validateNetworkCaps(m.Capabilities.Network); err != nil {
			return err
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
	if err := m.validateAuthCaps(); err != nil {
		return err
	}
	if err := m.validateTunnelCaps(); err != nil {
		return err
	}
	if err := m.validateChannelCaps(); err != nil {
		return err
	}
	if err := m.validateDiscoveryCaps(); err != nil {
		return err
	}
	return nil
}

func (m *Manifest) validateSessionCaps() error {
	if m.Capabilities.Session == nil {
		return nil
	}
	s := m.Capabilities.Session
	if s.Terminal && s.Embed {
		return fmt.Errorf("%w: terminal and embed are mutually exclusive", ErrInvalidManifest)
	}
	if s.Embed && s.AllowMultiSession {
		return fmt.Errorf("%w: embed plugins may not use allowMultiSession", ErrInvalidManifest)
	}
	if s.LocalEmbedServer && !s.Embed {
		return fmt.Errorf("%w: localEmbedServer requires embed", ErrInvalidManifest)
	}
	if s.RemoteFS && !s.Terminal && !s.Embed {
		return fmt.Errorf("%w: remoteFs requires terminal or embed session surface", ErrInvalidManifest)
	}
	if s.Embed {
		if len(s.ConnectProtocols) == 0 {
			return fmt.Errorf("%w: embed requires capabilities.session.connectProtocols", ErrInvalidManifest)
		}
		if m.EffectiveIsolation() != IsolationPerSession {
			return fmt.Errorf("%w: embed requires isolation per-session", ErrInvalidManifest)
		}
	}
	if s.AllowMultiSession {
		if s.Terminal {
			return fmt.Errorf("%w: terminal plugins may not use allowMultiSession", ErrInvalidManifest)
		}
		if m.EffectiveIsolation() != IsolationPerPlugin {
			return fmt.Errorf("%w: allowMultiSession requires per-plugin isolation", ErrInvalidManifest)
		}
	}
	if s.Terminal && m.EffectiveIsolation() != IsolationPerSession {
		return fmt.Errorf("%w: terminal requires isolation per-session", ErrInvalidManifest)
	}
	return nil
}

func allowedAuthMethodKind(kind string) bool {
	switch kind {
	case "keyboard-interactive", "publickey":
		return true
	default:
		return false
	}
}

func (m *Manifest) validateAuthCaps() error {
	if m.Capabilities.Auth == nil {
		return nil
	}
	a := m.Capabilities.Auth
	for _, method := range a.Methods {
		if !allowedAuthMethodKind(strings.TrimSpace(method)) {
			return fmt.Errorf("%w: invalid auth method %q", ErrInvalidManifest, method)
		}
	}
	if a.Provider && len(m.Contributions.AuthMethods) == 0 {
		return fmt.Errorf("%w: auth.provider requires contributions.authMethods", ErrInvalidManifest)
	}
	allowed := make(map[string]struct{}, len(a.Methods))
	for _, method := range a.Methods {
		allowed[strings.TrimSpace(method)] = struct{}{}
	}
	for _, am := range m.Contributions.AuthMethods {
		id := strings.TrimSpace(am.ID)
		if id == "" {
			return ErrInvalidManifest
		}
		kind := strings.TrimSpace(am.Kind)
		if !allowedAuthMethodKind(kind) {
			return fmt.Errorf("%w: invalid auth method kind %q", ErrInvalidManifest, kind)
		}
		if _, ok := allowed[kind]; !ok {
			return fmt.Errorf("%w: auth method %q not listed in capabilities.auth.methods", ErrInvalidManifest, kind)
		}
	}
	return nil
}

func (m *Manifest) validateTunnelCaps() error {
	if m.Capabilities.Tunnel == nil {
		return nil
	}
	if m.Capabilities.Tunnel.Provider && len(m.Contributions.TunnelProviders) == 0 {
		return fmt.Errorf("%w: tunnel.provider requires contributions.tunnelProviders", ErrInvalidManifest)
	}
	for _, tp := range m.Contributions.TunnelProviders {
		if strings.TrimSpace(tp.ID) == "" {
			return ErrInvalidManifest
		}
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

// SessionSurface returns "terminal", "embed", or "" for the plugin session UI type.
func (m *Manifest) SessionSurface() string {
	if m.Capabilities.Session == nil {
		return ""
	}
	if m.Capabilities.Session.Embed {
		return "embed"
	}
	if m.Capabilities.Session.Terminal {
		return "terminal"
	}
	return ""
}

// RequiresLocalEmbedServerWarning reports whether install should warn about loopback HTTP.
func (m *Manifest) RequiresLocalEmbedServerWarning() bool {
	return m.Capabilities.Session != nil && m.Capabilities.Session.LocalEmbedServer
}

// EmbedEntryForProtocol returns the ui entry for a connection protocol (embed plugins only).
func (m *Manifest) EmbedEntryForProtocol(protocolID string) string {
	protocolID = strings.TrimSpace(protocolID)
	for _, cp := range m.Contributions.ConnectionProtocols {
		if strings.TrimSpace(cp.ID) == protocolID {
			entry := strings.TrimSpace(cp.EmbedEntry)
			if entry == "" {
				return defaultEmbedEntry
			}
			return entry
		}
	}
	return defaultEmbedEntry
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
		if m.Capabilities.Session != nil && m.Capabilities.Session.Embed {
			entry := strings.TrimSpace(cp.EmbedEntry)
			if entry == "" {
				entry = defaultEmbedEntry
			}
			if err := ValidateViewAssetEntry(entry); err != nil {
				return err
			}
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

func validateNetworkCaps(n *NetworkCaps) error {
	if n.AllowPrivateNetworks && !n.AllowArbitraryOutbound {
		return fmt.Errorf("%w: allowPrivateNetworks requires allowArbitraryOutbound", ErrInvalidManifest)
	}
	for _, pattern := range n.Outbound {
		if err := validateNetworkPattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

// RequiresArbitraryNetworkAccess reports whether the manifest requests unrestricted outbound TCP.
func (m *Manifest) RequiresArbitraryNetworkAccess() bool {
	return m.Capabilities.Network != nil && m.Capabilities.Network.AllowArbitraryOutbound
}

// RequiresPrivateNetworkAccess reports whether arbitrary outbound includes private/LAN addresses.
func (m *Manifest) RequiresPrivateNetworkAccess() bool {
	return m.RequiresArbitraryNetworkAccess() && m.Capabilities.Network.AllowPrivateNetworks
}

// RequiresArbitraryNetworkWarning reports whether install should warn about unrestricted network access.
func (m *Manifest) RequiresArbitraryNetworkWarning() bool {
	return m.RequiresArbitraryNetworkAccess()
}

// HasNetworkCapability reports whether the manifest declares any outbound network permission.
func (m *Manifest) HasNetworkCapability() bool {
	if m.Capabilities.Network == nil {
		return false
	}
	n := m.Capabilities.Network
	return n.AllowArbitraryOutbound || len(n.Outbound) > 0
}

func allowedSecretField(field string) bool {
	switch field {
	case "password", "privateKey", "passphrase":
		return true
	default:
		return false
	}
}

// RequiresChannelExecConsent reports whether the plugin declared the exec channel purpose, which
// runs commands over the user's authenticated SSH session and therefore requires an explicit
// install-time grant, like auth.provider and allowArbitraryOutbound (ADR-011 D3). It is the
// predicate every install path gates on, not merely a line in the permission summary.
func (m *Manifest) RequiresChannelExecConsent() bool {
	if m.Capabilities.Channel == nil {
		return false
	}
	for _, p := range m.Capabilities.Channel.Purposes {
		if strings.TrimSpace(p) == PurposeExec {
			return true
		}
	}
	return false
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
	if m.Capabilities.Network != nil {
		n := m.Capabilities.Network
		if n.AllowArbitraryOutbound {
			line := "Outbound network: any public host and port (TCP)"
			if n.AllowPrivateNetworks {
				line += " (including private, loopback, and link-local addresses)"
			}
			lines = append(lines, line)
		} else if len(n.Outbound) > 0 {
			lines = append(lines, "Outbound network: "+strings.Join(n.Outbound, ", "))
		}
	}
	if m.Capabilities.Vault != nil && len(m.Capabilities.Vault.ReadConnectionFields) > 0 {
		lines = append(lines, "Read connection metadata (no secrets by default)")
	}
	if m.RequiresSecretAccess() {
		lines = append(lines, "Access connection secrets: "+strings.Join(m.Capabilities.Vault.GetSecret, ", "))
	}
	if m.RequiresAuthProviderAccess() {
		lines = append(lines, "Provide SSH authentication methods for connections that use this plugin")
	}
	if m.Capabilities.Tunnel != nil && m.Capabilities.Tunnel.Provider {
		lines = append(lines, "Route dynamic port-forward connections (tunnel provider)")
	}
	if m.RequiresMultiSessionWarning() {
		lines = append(lines, "Shared process may access multiple sessions (allowMultiSession)")
	}
	if m.Capabilities.Session != nil && m.Capabilities.Session.Embed {
		lines = append(lines, "Render session in embedded browser surface (embed)")
	}
	if m.RequiresLocalEmbedServerWarning() {
		lines = append(lines, "Run local HTTP server for session UI (loopback only)")
	}
	if m.RequiresChannelExecConsent() {
		lines = append(lines, "Run commands over your authenticated session (exec channel)")
	}
	if m.Capabilities.Discovery != nil {
		lines = append(lines, "Show discovered resources under your connections")
	}
	if len(lines) == 0 {
		lines = append(lines, "No elevated permissions requested")
	}
	return lines
}
