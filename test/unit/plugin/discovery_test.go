package plugin_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
	"ssh-client/internal/usecase"
)

func hashFileContents(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func writeChecksumsFile(dir string) error {
	var lines []string
	for _, name := range []string{"plugin.json", "p.exe"} {
		sum, err := hashFileContents(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		lines = append(lines, sum+"  "+name)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	return os.WriteFile(filepath.Join(dir, bundle.ChecksumsFile), []byte(content), 0o600)
}

func TestPluginTerminalWriteBackpressure(t *testing.T) {
	manager := usecase.NewSessionManager(usecase.SessionManagerConfig{
		PluginTerminalWriteTimeout: 50 * time.Millisecond,
	})
	sessionID := "sess-backpressure"
	if err := manager.BindPluginSessionForTest(sessionID, "plugin-a", 0); err != nil {
		t.Fatal(err)
	}

	go func() {
		_ = manager.HandlePluginWriteTerminal("plugin-a", sessionID, []byte("first"))
	}()

	err := manager.HandlePluginWriteTerminal("plugin-a", sessionID, []byte("blocked"))
	if err != domainplugin.ErrTerminalBackpressure {
		t.Fatalf("expected ErrTerminalBackpressure, got %v", err)
	}
}

func TestDiscoveryUserOverridesBundled(t *testing.T) {
	exeDir := t.TempDir()
	dataRoot := t.TempDir()
	pluginID := "com.test.discovery"

	writePluginDir := func(root, name, version string, userInstalled bool) {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{
			"id": "` + pluginID + `",
			"name": "Discovery",
			"version": "` + version + `",
			"engine": {"type": "go-binary", "entry": "p.exe"}
		}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeChecksumsFile(dir); err != nil {
			t.Fatal(err)
		}
		if userInstalled {
			if err := infraplugin.MarkUserInstalled(dir); err != nil {
				t.Fatal(err)
			}
		}
	}

	writePluginDir(filepath.Join(exeDir, "plugins"), "bundled", "1.0.0", false)
	writePluginDir(filepath.Join(dataRoot, "plugins"), "user", "2.0.0", true)

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(exeDir, dataRoot))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Version != "2.0.0" {
		t.Fatalf("expected user override version 2.0.0, got %s", plugins[0].Manifest.Version)
	}
	if plugins[0].Source != domainplugin.SourceUser {
		t.Fatalf("expected user source, got %q", plugins[0].Source)
	}
}

func TestDiscoveryFallsBackToExePluginsWhenDataEmpty(t *testing.T) {
	exeDir := t.TempDir()
	dataRoot := t.TempDir()
	pluginID := "com.test.fallback"

	dir := filepath.Join(exeDir, "plugins", "bundled")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "` + pluginID + `",
		"name": "Fallback",
		"version": "1.0.0",
		"engine": {"type": "go-binary", "entry": "p.exe"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksumsFile(dir); err != nil {
		t.Fatal(err)
	}

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(exeDir, dataRoot))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected bundled fallback plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Version != "1.0.0" {
		t.Fatalf("unexpected version %s", plugins[0].Manifest.Version)
	}
}

func TestDiscoveryRejectsTamperedUserPlugin(t *testing.T) {
	dataRoot := t.TempDir()
	pluginID := "com.test.tamper"
	dir := filepath.Join(dataRoot, "plugins", "tampered")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "` + pluginID + `",
		"name": "Tamper",
		"version": "1.0.0",
		"engine": {"type": "go-binary", "entry": "p.exe"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("original"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksumsFile(dir); err != nil {
		t.Fatal(err)
	}
	if err := infraplugin.MarkUserInstalled(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := infraplugin.LoadPluginDir(dir); err == nil {
		t.Fatal("expected tampered user plugin to be rejected")
	}

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths("", dataRoot))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected no plugins after tamper, got %d", len(plugins))
	}
}

func TestDiscoverySkipsIncompatibleMinCoreVersion(t *testing.T) {
	exeDir := t.TempDir()
	dir := filepath.Join(exeDir, "plugins", "future")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "com.test.future",
		"name": "Future",
		"version": "1.0.0",
		"minCoreVersion": "99.0.0",
		"engine": {"type": "go-binary", "entry": "p.exe"}
	}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksumsFile(dir); err != nil {
		t.Fatal(err)
	}

	discovery := infraplugin.NewDiscovery(infraplugin.SearchPaths(exeDir, ""))
	plugins, err := discovery.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected incompatible plugin skipped, got %d", len(plugins))
	}
}
