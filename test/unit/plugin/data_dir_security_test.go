package plugin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

func TestPluginInstanceDataDirPerSession(t *testing.T) {
	root := t.TempDir()
	const (
		pluginID  = "com.test.session"
		sessionID = "sess-abc"
	)

	perPlugin := infraplugin.PluginInstanceDataDir(root, pluginID, "", domainplugin.IsolationPerPlugin)
	perSession := infraplugin.PluginInstanceDataDir(root, pluginID, sessionID, domainplugin.IsolationPerSession)

	if perPlugin == perSession {
		t.Fatalf("expected different dirs for per-session isolation, got %q", perSession)
	}
	if filepath.Base(perSession) != sessionID {
		t.Fatalf("expected session dir basename %q, got %q", sessionID, filepath.Base(perSession))
	}
}

func TestFSProxyUsesPerSessionDataDir(t *testing.T) {
	root := t.TempDir()
	const (
		pluginID  = "com.test.session-fs"
		sessionID = "sess-fs-1"
	)

	sessionDir, err := infraplugin.EnsurePluginInstanceDataDir(root, pluginID, sessionID, domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatal(err)
	}
	pluginDir, err := infraplugin.EnsurePluginInstanceDataDir(root, pluginID, "", domainplugin.IsolationPerPlugin)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sessionDir, "session-only.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "shared.txt"), []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}

	sessionFS, err := capability.NewFSProxy(&domainplugin.FSCaps{Read: []string{"${pluginData}"}}, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	pluginFS, err := capability.NewFSProxy(&domainplugin.FSCaps{Read: []string{"${pluginData}"}}, pluginDir)
	if err != nil {
		t.Fatal(err)
	}

	sessionParams, _ := json.Marshal(map[string]string{"path": "session-only.txt"})
	if _, err := sessionFS.Handle("fs.read", sessionParams); err != nil {
		t.Fatalf("session fs.read failed: %v", err)
	}

	outsideParams, _ := json.Marshal(map[string]string{"path": "../shared.txt"})
	if _, err := sessionFS.Handle("fs.read", outsideParams); err == nil {
		t.Fatal("expected session fs.read to reject plugin-level file")
	}

	pluginParams, _ := json.Marshal(map[string]string{"path": "shared.txt"})
	if _, err := pluginFS.Handle("fs.read", pluginParams); err != nil {
		t.Fatalf("plugin fs.read failed: %v", err)
	}
}

func TestManifestRequiresConnectProtocolsForContributions(t *testing.T) {
	raw := `{
		"id":"com.test.protocol",
		"name":"Test",
		"version":"1.0.0",
		"engine":{"type":"go-binary","entry":"p.exe"},
		"contributions":{"connectionProtocols":[{"id":"telnet","label":"Telnet"}]}
	}`
	var m domainplugin.Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected manifest validation to require connectProtocols")
	}

	m.Capabilities.Session = &domainplugin.SessionCaps{
		ConnectProtocols: []string{"telnet"},
		Terminal:         true,
	}
	m.Isolation = domainplugin.IsolationPerSession
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}
	if !m.AllowsConnectProtocol("telnet") {
		t.Fatal("expected telnet to be allowed")
	}
	if m.AllowsConnectProtocol("ssh") {
		t.Fatal("expected ssh to be denied")
	}
}
