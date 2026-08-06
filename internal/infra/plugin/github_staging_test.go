package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

func stagingTestManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.example.demo",
		Name:    "Demo",
		Version: "1.0.0",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "demo.exe"},
	}
}

// packDemoBundle writes a plugin tree with a ui/ asset and packs it, returning the bundle path
// and the tree it was packed from.
func packDemoBundle(t *testing.T, manifest domainplugin.Manifest) (bundlePath, sourceDir string) {
	t.Helper()
	sourceDir = t.TempDir()

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	writeFile(t, filepath.Join(sourceDir, "plugin.json"), data)
	writeFile(t, filepath.Join(sourceDir, manifest.Engine.Entry), []byte("binary"))
	writeFile(t, filepath.Join(sourceDir, "ui", "demo.html"), []byte("<html></html>"))
	writeFile(t, filepath.Join(sourceDir, "ui", "vendor", "lib.js"), []byte("export {}"))

	bundlePath = filepath.Join(t.TempDir(), "demo-windows-amd64.xqsp")
	if err := bundle.Pack(sourceDir, bundlePath); err != nil {
		t.Fatalf("pack bundle: %v", err)
	}
	return bundlePath, sourceDir
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestStageGitHubPluginFromBinaryWritesManifestAndChecksums(t *testing.T) {
	manifest := stagingTestManifest()
	binary := filepath.Join(t.TempDir(), "downloaded")
	writeFile(t, binary, []byte("binary"))

	staged, cleanup, err := stageGitHubPlugin(t.TempDir(),
		domainplugin.DownloadedAsset{Path: binary, Kind: domainplugin.ReleaseAssetBinary, AssetName: "demo-windows-amd64.exe"},
		manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	for _, name := range []string{manifest.Engine.Entry, "plugin.json", bundle.ChecksumsFile} {
		if _, err := os.Stat(filepath.Join(staged.Dir, name)); err != nil {
			t.Fatalf("expected %s in the staging dir: %v", name, err)
		}
	}
	if staged.Manifest.ID != manifest.ID {
		t.Fatalf("staged manifest = %q, want %q", staged.Manifest.ID, manifest.ID)
	}
}

func TestStageGitHubPluginFromBundleKeepsTheAuthorsTree(t *testing.T) {
	manifest := stagingTestManifest()
	bundlePath, sourceDir := packDemoBundle(t, manifest)

	staged, cleanup, err := stageGitHubPlugin(t.TempDir(),
		domainplugin.DownloadedAsset{Path: bundlePath, Kind: domainplugin.ReleaseAssetBundle, AssetName: filepath.Base(bundlePath)},
		// A different repository manifest on purpose: the bundle's own plugin.json is what must
		// land on disk and what must be reported back.
		domainplugin.Manifest{ID: "com.example.demo", Version: "9.9.9"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(staged.Dir, "ui", "demo.html")); err != nil {
		t.Fatalf("expected the bundle's ui/ assets to be staged: %v", err)
	}
	if _, err := os.Stat(filepath.Join(staged.Dir, "ui", "vendor", "lib.js")); err != nil {
		t.Fatalf("expected nested ui/ assets to be staged: %v", err)
	}
	if staged.Manifest.Version != manifest.Version {
		t.Fatalf("staged manifest version = %q, want the bundle's %q", staged.Manifest.Version, manifest.Version)
	}

	// The author's checksums and manifest must survive byte for byte: the signature, when there
	// is one, is bound to their digest.
	for _, name := range []string{bundle.ChecksumsFile, "plugin.json"} {
		if !bytes.Equal(readFile(t, filepath.Join(staged.Dir, name)), readBundleEntry(t, bundlePath, name)) {
			t.Fatalf("%s was rewritten during staging", name)
		}
	}
	if !bytes.Equal(readFile(t, filepath.Join(staged.Dir, "plugin.json")), readFile(t, filepath.Join(sourceDir, "plugin.json"))) {
		t.Fatal("the staged manifest differs from the one the author packed")
	}
}

func TestStageGitHubPluginFromBundleRejectsATamperedTree(t *testing.T) {
	manifest := stagingTestManifest()
	bundlePath, _ := packDemoBundle(t, manifest)
	rewriteBundleEntry(t, bundlePath, "ui/demo.html", []byte("<html>tampered</html>"))

	_, _, err := stageGitHubPlugin(t.TempDir(),
		domainplugin.DownloadedAsset{Path: bundlePath, Kind: domainplugin.ReleaseAssetBundle, AssetName: filepath.Base(bundlePath)},
		manifest)
	if err == nil {
		t.Fatal("expected staging to reject a bundle whose contents do not match SHA256SUMS")
	}
}

func TestStageGitHubPluginFromBundleRequiresChecksums(t *testing.T) {
	manifest := stagingTestManifest()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	bundlePath := filepath.Join(t.TempDir(), "demo-windows-amd64.xqsp")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string][]byte{"plugin.json": data, manifest.Engine.Entry: []byte("binary")} {
		w, createErr := zw.Create(name)
		if createErr != nil {
			t.Fatalf("create zip entry: %v", createErr)
		}
		if _, writeErr := w.Write(content); writeErr != nil {
			t.Fatalf("write zip entry: %v", writeErr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	writeFile(t, bundlePath, buf.Bytes())

	_, _, err = stageGitHubPlugin(t.TempDir(),
		domainplugin.DownloadedAsset{Path: bundlePath, Kind: domainplugin.ReleaseAssetBundle, AssetName: filepath.Base(bundlePath)},
		manifest)
	if !errors.Is(err, bundle.ErrMissingChecksums) {
		t.Fatalf("expected ErrMissingChecksums, got %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return data
}

func readBundleEntry(t *testing.T, bundlePath, name string) []byte {
	t.Helper()
	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			t.Fatalf("open bundle entry %q: %v", name, openErr)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, copyErr := buf.ReadFrom(rc); copyErr != nil {
			t.Fatalf("read bundle entry %q: %v", name, copyErr)
		}
		return buf.Bytes()
	}
	t.Fatalf("bundle entry %q not found", name)
	return nil
}

func rewriteBundleEntry(t *testing.T, bundlePath, name string, content []byte) {
	t.Helper()
	r, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range r.File {
		w, createErr := zw.Create(f.Name)
		if createErr != nil {
			t.Fatalf("create zip entry: %v", createErr)
		}
		data := content
		if f.Name != name {
			rc, openErr := f.Open()
			if openErr != nil {
				t.Fatalf("open zip entry: %v", openErr)
			}
			var entry bytes.Buffer
			if _, copyErr := entry.ReadFrom(rc); copyErr != nil {
				t.Fatalf("read zip entry: %v", copyErr)
			}
			rc.Close()
			data = entry.Bytes()
		}
		if _, writeErr := w.Write(data); writeErr != nil {
			t.Fatalf("write zip entry: %v", writeErr)
		}
	}
	r.Close()
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	writeFile(t, bundlePath, buf.Bytes())
}
