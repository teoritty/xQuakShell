package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

func TestSafePluginInstallDirRejectsTraversalID(t *testing.T) {
	dataRoot := t.TempDir()
	_, err := infraplugin.SafePluginInstallDir(dataRoot, "..")
	if err == nil {
		t.Fatal("expected rejection for .. id")
	}
}

func TestInstallBundleUsesSafePath(t *testing.T) {
	dataRoot := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"com.test.install","name":"T","version":"1","engine":{"type":"go-binary","entry":"p.exe"}}`
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "p.exe"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(src); err != nil {
		t.Fatal(err)
	}

	installed, err := infraplugin.InstallBundle(src, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Manifest.ID != "com.test.install" {
		t.Fatalf("unexpected id %q", installed.Manifest.ID)
	}
	want := filepath.Join(dataRoot, "plugins", "com.test.install")
	if installed.RootDir != want {
		t.Fatalf("root dir %q want %q", installed.RootDir, want)
	}
}

func TestLoadPluginDirRejectsEngineEntryOutsideBundle(t *testing.T) {
	parent := t.TempDir()
	pluginDir := filepath.Join(parent, "plugin")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Place a binary outside the plugin bundle — a malicious manifest could target it.
	outside := filepath.Join(parent, "outside.exe")
	if err := os.WriteFile(outside, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}

	manifest := `{
		"id":"com.test.traversal",
		"name":"Traversal",
		"version":"1.0.0",
		"engine":{"type":"go-binary","entry":"../outside.exe"}
	}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := infraplugin.LoadPluginDir(pluginDir)
	if err == nil {
		t.Fatal("expected load to reject engine.entry outside plugin bundle")
	}
}
