package plugin

import "testing"

func TestAllowsConnectProtocol(t *testing.T) {
	m := Manifest{
		Capabilities: CapabilitySet{
			Session: &SessionCaps{
				ConnectProtocols: []string{"telnet", "demo-terminal"},
				Terminal:         true,
			},
		},
	}
	if !m.AllowsConnectProtocol("telnet") {
		t.Fatal("expected telnet allowed")
	}
	if m.AllowsConnectProtocol("ssh") {
		t.Fatal("expected ssh denied")
	}
}

func TestValidateConnectionProtocolCapsRequiresDeclaration(t *testing.T) {
	m := Manifest{
		Contributions: Contributions{
			ConnectionProtocols: []ConnectionProtocolContribution{
				{ID: "telnet", Label: "Telnet"},
			},
		},
	}
	if err := m.validateConnectionProtocolCaps(); err == nil {
		t.Fatal("expected missing session caps error")
	}

	m.Capabilities.Session = &SessionCaps{ConnectProtocols: []string{"rdp"}}
	if err := m.validateConnectionProtocolCaps(); err == nil {
		t.Fatal("expected protocol mismatch error")
	}

	m.Capabilities.Session.ConnectProtocols = []string{"telnet"}
	if err := m.validateConnectionProtocolCaps(); err != nil {
		t.Fatalf("expected valid caps, got %v", err)
	}
}

func TestTerminalRequiresPerSessionUnlessAllowMultiSession(t *testing.T) {
	m := Manifest{
		ID:      "com.test.terminal",
		Name:    "T",
		Version: "1.0.0",
		Engine:  EngineConfig{Type: EngineGoBinary, Entry: "p.exe"},
		Capabilities: CapabilitySet{
			Session: &SessionCaps{
				ConnectProtocols: []string{"demo"},
				Terminal:         true,
			},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected terminal+per-plugin default to fail validation")
	}
	m.Isolation = IsolationPerSession
	if err := m.Validate(); err != nil {
		t.Fatalf("expected per-session terminal ok, got %v", err)
	}
	m.Isolation = IsolationPerPlugin
	m.Capabilities.Session.AllowMultiSession = true
	if err := m.Validate(); err == nil {
		t.Fatal("expected terminal+allowMultiSession to fail validation")
	}
	if m.RequiresMultiSessionWarning() {
		t.Fatal("terminal plugin should not trigger multi-session warning")
	}
}

func TestAllowMultiSessionWarningForNonTerminal(t *testing.T) {
	m := Manifest{
		ID:      "com.test.multi",
		Name:    "M",
		Version: "1.0.0",
		Engine:  EngineConfig{Type: EngineGoBinary, Entry: "p.exe"},
		Capabilities: CapabilitySet{
			Session: &SessionCaps{
				AllowMultiSession: true,
			},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected non-terminal allowMultiSession ok, got %v", err)
	}
	if !m.RequiresMultiSessionWarning() {
		t.Fatal("expected multi-session install warning")
	}
	trust, err := EvaluateInstallTrust(m, InstallTrustPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if !trust.MultiSessionWarning {
		t.Fatal("expected trust multi-session warning")
	}
}

func TestAllowMultiSessionRequiresPerPluginIsolation(t *testing.T) {
	m := Manifest{
		ID:        "com.test.multi",
		Name:      "M",
		Version:   "1.0.0",
		Engine:    EngineConfig{Type: EngineGoBinary, Entry: "p.exe"},
		Isolation: IsolationPerSession,
		Capabilities: CapabilitySet{
			Session: &SessionCaps{AllowMultiSession: true},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected allowMultiSession with per-session isolation to fail")
	}
}
