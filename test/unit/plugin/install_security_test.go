package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

func TestSafePluginInstallDirRejectsTraversalID(t *testing.T) {
	dataRoot := t.TempDir()
	_, err := infraplugin.SafePluginInstallDir(dataRoot, "..")
	if err == nil {
		t.Fatal("expected rejection for .. id")
	}
}

func TestInstallFromSourceUsesSafePath(t *testing.T) {
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

	installed, err := infraplugin.InstallFromSource(src, dataRoot)
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

// TestInstallFromSourceRefusesTreeWithoutChecksums pins the property that removing the second
// install helper established: there is one install path, and it validates. The deleted
// InstallBundle copied this exact tree into place and reported success, because it never called
// loadSource — an install route that skipped every check the other one performed.
func TestInstallFromSourceRefusesTreeWithoutChecksums(t *testing.T) {
	dataRoot := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"com.test.nosums","name":"N","version":"1","engine":{"type":"go-binary","entry":"p.exe"}}`
	if err := os.WriteFile(filepath.Join(src, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "p.exe"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Deliberately no bundle.WriteChecksums: the tree makes no claim about its own contents.

	if _, err := infraplugin.InstallFromSource(src, dataRoot); err == nil {
		t.Fatal("InstallFromSource = nil, want error; a tree carrying no SHA256SUMS must not install")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "plugins", "com.test.nosums")); !os.IsNotExist(err) {
		t.Fatalf("refused install left a tree behind at plugins/com.test.nosums (stat err = %v)", err)
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
