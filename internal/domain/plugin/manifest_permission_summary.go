package plugin

import "strings"

// The human-readable permission lines shown on the install screen.
//
// They live apart from manifest_capabilities.go because they answer a different question. That file
// decides whether a manifest is well formed and refuses it when it is not; this one turns a
// well-formed manifest into sentences for a person deciding whether to trust it. One file was
// carrying both a machine contract and a piece of the interface.

// PermissionSummary returns human-readable install-time permission lines.
func (m *Manifest) PermissionSummary() []string {
	var lines []string
	lines = append(lines, m.filesystemPermissionLines()...)
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
	if m.RequiresChannelExecConsent() {
		lines = append(lines, "Run commands over your authenticated session (exec channel)")
	}
	if m.Capabilities.Discovery != nil {
		lines = append(lines, "Show discovered resources under your connections")
	}
	if m.Capabilities.UI != nil {
		lines = append(lines, "Show its own tabs, dialogs and node details")
	}
	if len(lines) == 0 {
		lines = append(lines, "No elevated permissions requested")
	}
	return lines
}
