package plugin_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infracache "xquakshell/internal/infra/cache"
	infrapersistence "xquakshell/internal/infra/persistence"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/usecase"
)

// bundleAssetName builds a .xqsp asset name matching runtime.GOOS/GOARCH, so
// GetPlatformForCurrent resolves it on any CI platform. Its bare-binary sibling is
// currentPlatformAssetName.
func bundleAssetName() string {
	return "demo-" + runtime.GOOS + "-" + runtime.GOARCH + ".xqsp"
}

func writeFixtureFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

// bundleDownloader serves one prepacked release asset and records what was asked for.
type bundleDownloader struct {
	assetPath  string
	calls      []domainplugin.AssetDownloadRequest
	failOnCall bool
}

func (d *bundleDownloader) DownloadAsset(_ context.Context, req domainplugin.AssetDownloadRequest) (domainplugin.DownloadedAsset, func(), error) {
	d.calls = append(d.calls, req)
	if d.failOnCall {
		return domainplugin.DownloadedAsset{}, func() {}, errors.New("download disabled in test")
	}
	return domainplugin.DownloadedAsset{
		Path:      d.assetPath,
		Kind:      domainplugin.ClassifyReleaseAsset(req.AssetName),
		AssetName: req.AssetName,
	}, func() {}, nil
}

func (d *bundleDownloader) DownloadAssetContent(_ context.Context, _, _, _, _ string) ([]byte, error) {
	return nil, errors.New("no checksum asset in test")
}

// installRig wires the real stager, loader and installer around a fake GitHub.
type installRig struct {
	service    *usecase.GitHubPluginService
	downloader *bundleDownloader
	dataRoot   string
}

func newInstallRig(t *testing.T, repoManifest string, assets []domainplugin.GitHubReleaseAsset, downloader *bundleDownloader) installRig {
	t.Helper()
	dataRoot := t.TempDir()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:      usecase.NewPluginRegistry(),
		LoadBundle:    infraplugin.LoadPluginSource,
		InstallBundle: infraplugin.InstallFromSource,
		InstallRoot:   dataRoot,
	})

	storage, err := infrapersistence.NewFileGitHubRepositoryStorage(t.TempDir())
	if err != nil {
		t.Fatalf("build repo storage: %v", err)
	}
	if err := storage.Add(context.Background(), domainplugin.GitHubRepository{
		URL:         "https://github.com/user/repo",
		Owner:       "user",
		Repo:        "repo",
		DisplayName: "user/repo",
		AddedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("register repo: %v", err)
	}

	client := &recordingGitHubClient{
		manifest: []byte(repoManifest),
		releases: []domainplugin.GitHubRelease{{TagName: "v1.0.0", Assets: assets}},
	}
	service := usecase.NewGitHubPluginService(
		client,
		downloader,
		infraplugin.NewGitHubPluginStager(t.TempDir()),
		infraplugin.InstallMetaWriter{},
		infracache.NewMemoryCache(domainplugin.DefaultCacheTTL),
		manager,
		storage,
	)
	return installRig{service: service, downloader: downloader, dataRoot: dataRoot}
}

func (r installRig) install(t *testing.T) error {
	t.Helper()
	return r.service.InstallPluginFromGitHub(context.Background(),
		"https://github.com/user/repo", "v1.0.0", false, false, false, false, false, false)
}

const embedPluginManifest = `{
  "id": "com.example.demo",
  "name": "Demo Plugin",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"},
  "isolation": "per-session",
  "capabilities": {"session": {"connectProtocols": ["demo"], "embed": true}},
  "contributions": {"connectionProtocols": [{"id": "demo", "label": "Demo", "embedEntry": "ui/demo.html"}]}
}`

func TestInstallFromGitHubKeepsTheBundlesUIAssets(t *testing.T) {
	assetPath := packBundle(t, embedPluginManifest, "ui/demo.html")
	rig := newInstallRig(t, embedPluginManifest,
		[]domainplugin.GitHubReleaseAsset{{Name: bundleAssetName()}},
		&bundleDownloader{assetPath: assetPath})

	if err := rig.install(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	installed := filepath.Join(rig.dataRoot, "plugins", "com.example.demo")
	if _, err := os.Stat(filepath.Join(installed, "ui", "demo.html")); err != nil {
		t.Fatalf("expected the plugin's ui/ assets to be installed: %v", err)
	}
	if !bytes.Equal(readFile(t, filepath.Join(installed, bundle.ChecksumsFile)), bundleEntry(t, assetPath, bundle.ChecksumsFile)) {
		t.Fatal("the installed SHA256SUMS is not the one the author packed")
	}
}

func TestInstallFromGitHubRejectsABundleWithAnotherID(t *testing.T) {
	otherManifest := `{
  "id": "com.attacker.other",
  "name": "Other",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"}
}`
	assetPath := packBundle(t, otherManifest)
	rig := newInstallRig(t, embedPluginManifest,
		[]domainplugin.GitHubReleaseAsset{{Name: bundleAssetName()}},
		&bundleDownloader{assetPath: assetPath})

	err := rig.install(t)
	if !errors.Is(err, domainplugin.ErrBundleIdentityMismatch) {
		t.Fatalf("expected ErrBundleIdentityMismatch, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(rig.dataRoot, "plugins", "com.attacker.other")); statErr == nil {
		t.Fatal("a mismatched bundle was installed")
	}
}

func TestInstallFromGitHubAcceptsAVersionOlderThanTheRepository(t *testing.T) {
	// The repository manifest is fetched from the default branch when a tag carries none, so an
	// older tag legitimately disagrees on version. That must not fail the install.
	repoManifest := `{
  "id": "com.example.demo",
  "name": "Demo Plugin",
  "version": "9.9.9",
  "engine": {"type": "go-binary", "entry": "demo.exe"},
  "isolation": "per-session",
  "capabilities": {"session": {"connectProtocols": ["demo"], "embed": true}},
  "contributions": {"connectionProtocols": [{"id": "demo", "label": "Demo", "embedEntry": "ui/demo.html"}]}
}`
	assetPath := packBundle(t, embedPluginManifest, "ui/demo.html")
	rig := newInstallRig(t, repoManifest,
		[]domainplugin.GitHubReleaseAsset{{Name: bundleAssetName()}},
		&bundleDownloader{assetPath: assetPath})

	if err := rig.install(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	installedManifest := readInstalledManifest(t, filepath.Join(rig.dataRoot, "plugins", "com.example.demo"))
	if installedManifest.Version != "1.0.0" {
		t.Fatalf("installed version = %q, want the bundle's 1.0.0", installedManifest.Version)
	}
}

func TestInstallFromGitHubDownloadsTheBundleWhenBothShapesArePublished(t *testing.T) {
	assetPath := packBundle(t, embedPluginManifest, "ui/demo.html")
	downloader := &bundleDownloader{assetPath: assetPath}
	rig := newInstallRig(t, embedPluginManifest, []domainplugin.GitHubReleaseAsset{
		{Name: currentPlatformAssetName()},
		{Name: bundleAssetName()},
	}, downloader)

	if err := rig.install(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(downloader.calls) != 1 {
		t.Fatalf("expected exactly one download, got %d", len(downloader.calls))
	}
	if downloader.calls[0].AssetName != bundleAssetName() {
		t.Fatalf("downloaded %q, want the bundle %q", downloader.calls[0].AssetName, bundleAssetName())
	}
}

func packBundle(t *testing.T, manifestJSON string, uiAssets ...string) string {
	t.Helper()
	src := t.TempDir()

	var manifest domainplugin.Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	writeFixtureFile(t, filepath.Join(src, "plugin.json"), []byte(manifestJSON))
	writeFixtureFile(t, filepath.Join(src, manifest.Engine.Entry), []byte("stub"))
	for _, asset := range uiAssets {
		writeFixtureFile(t, filepath.Join(src, filepath.FromSlash(asset)), []byte("<html></html>"))
	}

	out := filepath.Join(t.TempDir(), bundleAssetName())
	if err := bundle.Pack(src, out); err != nil {
		t.Fatalf("pack bundle: %v", err)
	}
	return out
}

// bundleEntry returns one file's bytes from inside a packed bundle.
func bundleEntry(t *testing.T, bundlePath, name string) []byte {
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

func readInstalledManifest(t *testing.T, dir string) domainplugin.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatalf("read installed manifest: %v", err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode installed manifest: %v", err)
	}
	return manifest
}
