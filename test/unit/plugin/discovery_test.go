package plugin_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/usecase"
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

func TestDiscoverySkipsIncompatiblePluginAPI(t *testing.T) {
	exeDir := t.TempDir()
	dir := filepath.Join(exeDir, "plugins", "future")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "com.test.future",
		"name": "Future",
		"version": "1.0.0",
		"requires": {"pluginApi": "99.0.0"},
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

// writeUserPluginDir lays out a user-installed plugin exactly as an install leaves it: manifest,
// binary, SHA256SUMS, and the marker that says a user put it there.
func writeUserPluginDir(t *testing.T, pluginID string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "plugins", "p")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"id": "` + pluginID + `",
		"name": "Plugin",
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
	return dir
}

// TestDiscoveryRejectsTamperedUserPlugin covers editing a file. This covers deleting the list it
// would have been checked against, which used to be the way to switch the check off entirely:
// verifyPluginIntegrity fell through to nil whenever the tree simply had no SHA256SUMS, so anyone
// who could edit the binary could also remove the evidence and the plugin loaded silently.
func TestDiscoveryRejectsAUserPluginWithItsChecksumsDeleted(t *testing.T) {
	dir := writeUserPluginDir(t, "com.test.stripped")

	if err := os.Remove(filepath.Join(dir, "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.exe"), []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := infraplugin.LoadPluginDir(dir); err == nil {
		t.Fatal("a user-installed plugin loaded after its SHA256SUMS was deleted; removing the evidence disabled the check")
	}
}

// Deleting the checksums is refused even when nothing else was touched. The file's absence is the
// contradiction - an install always writes one - so it does not need a second symptom to be wrong.
func TestDiscoveryRejectsAUserPluginWithNoChecksumsAtAll(t *testing.T) {
	dir := writeUserPluginDir(t, "com.test.nosums")

	if err := os.Remove(filepath.Join(dir, "SHA256SUMS")); err != nil {
		t.Fatal(err)
	}

	if _, err := infraplugin.LoadPluginDir(dir); err == nil {
		t.Fatal("a user-installed plugin with no SHA256SUMS loaded")
	}
}

// The ordinary case has to keep working, or every installed plugin stops loading.
func TestDiscoveryAcceptsAnIntactUserPlugin(t *testing.T) {
	dir := writeUserPluginDir(t, "com.test.intact")

	if _, err := infraplugin.LoadPluginDir(dir); err != nil {
		t.Fatalf("LoadPluginDir on an intact user plugin = %v, want nil", err)
	}
}
