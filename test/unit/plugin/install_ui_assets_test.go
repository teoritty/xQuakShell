package plugin_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

// writePluginTree lays out a plugin directory with checksums, omitting any ui/ asset the manifest
// declares unless it is listed in assets.
func writePluginTree(t *testing.T, manifestJSON string, entry string, assets ...string) string {
	t.Helper()
	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, "plugin.json"), []byte(manifestJSON))
	writeFixtureFile(t, filepath.Join(dir, entry), []byte("stub"))
	for _, asset := range assets {
		writeFixtureFile(t, filepath.Join(dir, filepath.FromSlash(asset)), []byte("x"))
	}
	if err := bundle.WriteChecksums(dir); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
	return dir
}

const viewPluginManifest = `{
  "id": "com.example.view",
  "name": "View Plugin",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"},
  "contributions": {"views": [{"id": "panel", "location": "sidebar", "title": "Panel", "entry": "ui/panel.html"}]}
}`

const iconPluginManifest = `{
  "id": "com.example.icons",
  "name": "Icon Plugin",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"},
  "capabilities": {"discovery": {"parentProtocols": ["ssh"]}},
  "contributions": {"discoveryIcons": [{"id": "box", "asset": "ui/icons/box.svg"}]}
}`

func TestInstallRejectsATreeMissingADeclaredUIAsset(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
	}{
		{name: "view entry", manifest: viewPluginManifest},
		{name: "embed entry", manifest: embedPluginManifest},
		{name: "discovery icon", manifest: iconPluginManifest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := writePluginTree(t, tc.manifest, "demo.exe")

			_, err := infraplugin.InstallFromSource(src, t.TempDir())
			if !errors.Is(err, domainplugin.ErrUIAssetMissing) {
				t.Fatalf("expected ErrUIAssetMissing, got %v", err)
			}
		})
	}
}

func TestInstallAcceptsATreeThatShipsItsDeclaredUIAssets(t *testing.T) {
	src := writePluginTree(t, viewPluginManifest, "demo.exe", "ui/panel.html")

	installed, err := infraplugin.InstallFromSource(src, t.TempDir())
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if installed.Manifest.ID != "com.example.view" {
		t.Fatalf("unexpected id %q", installed.Manifest.ID)
	}
}

// The refusal has to reach the user before consent, not after: preview is where the install
// dialog decides what to show.
func TestPreviewRejectsATreeMissingADeclaredUIAsset(t *testing.T) {
	src := writePluginTree(t, viewPluginManifest, "demo.exe")

	if _, err := infraplugin.LoadPluginSource(src); !errors.Is(err, domainplugin.ErrUIAssetMissing) {
		t.Fatalf("expected ErrUIAssetMissing, got %v", err)
	}
}

// An install made before this check existed must keep loading. A plugin that fails to load is not
// in the registry, and uninstall resolves through the registry — refusing it at load time would
// leave the user unable to remove it from the UI.
func TestLoadPluginDirStillLoadsAnInstalledTreeMissingAUIAsset(t *testing.T) {
	dir := writePluginTree(t, viewPluginManifest, "demo.exe")
	if err := infraplugin.MarkUserInstalled(dir); err != nil {
		t.Fatalf("mark user installed: %v", err)
	}

	installed, err := infraplugin.LoadPluginDir(dir)
	if err != nil {
		t.Fatalf("expected an installed plugin with a missing asset to keep loading, got %v", err)
	}
	if installed.Manifest.ID != "com.example.view" {
		t.Fatalf("unexpected id %q", installed.Manifest.ID)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "ui", "panel.html")); statErr == nil {
		t.Fatal("the fixture was supposed to omit the declared asset")
	}
}
