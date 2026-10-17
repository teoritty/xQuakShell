package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infracache "xquakshell/internal/infra/cache"
	infrapersistence "xquakshell/internal/infra/persistence"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/bundle"
	"xquakshell/internal/infra/portable"
	"xquakshell/internal/usecase"
)

const offerRepoURL = "https://github.com/user/repo"

const offerManifest = `{
  "id": "com.example.offered",
  "name": "Offered",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"}
}`

// offerRig is an install rig whose repository is NOT registered: a plugin offered by another
// device names a repository this device may never have added.
type offerRig struct {
	service  *usecase.GitHubPluginService
	registry *usecase.PluginRegistry
	storage  domainplugin.GitHubRepositoryStorage
	dataRoot string
	asset    string
	manifest string
}

func newOfferRig(t *testing.T) offerRig { return newOfferRigFor(t, offerManifest) }

func newOfferRigFor(t *testing.T, manifest string) offerRig {
	t.Helper()
	dataRoot := t.TempDir()
	registry := usecase.NewPluginRegistry()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry: registry, Host: idleProcessHost{},
		LoadBundle: infraplugin.LoadPluginSource, InstallBundle: infraplugin.InstallFromSource, InstallRoot: dataRoot,
	})
	storage, err := infrapersistence.NewFileGitHubRepositoryStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	asset := packBundle(t, manifest)
	client := &recordingGitHubClient{manifest: []byte(manifest),
		releases: []domainplugin.GitHubRelease{{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: bundleAssetName()}}}}}
	service := usecase.NewGitHubPluginService(client, &bundleDownloader{assetPath: asset},
		infraplugin.NewGitHubPluginStager(t.TempDir()), infraplugin.InstallMetaWriter{},
		infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), manager, storage)
	return offerRig{service: service, registry: registry, storage: storage, dataRoot: dataRoot, asset: asset, manifest: manifest}
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// expected is the build another device recorded: the digests of the bundle this rig serves.
func (r offerRig) expected(t *testing.T) usecase.ExpectedInstall {
	return usecase.ExpectedInstall{RepoURL: offerRepoURL, Tag: "v1.0.0",
		ManifestDigest: hexSHA256([]byte(r.manifest)),
		SumsDigest:     hexSHA256(bundleEntry(t, r.asset, bundle.ChecksumsFile))}
}

func (r offerRig) installedDir() string {
	return filepath.Join(r.dataRoot, "plugins", "com.example.offered")
}

func TestInstallExpectedInstallsTheRecordedBuild(t *testing.T) {
	rig := newOfferRig(t)
	want := rig.expected(t)
	if err := rig.service.InstallExpected(context.Background(), want); err != nil {
		t.Fatalf("install of the recorded build: %v", err)
	}
	got, err := rig.registry.Get("com.example.offered")
	if err != nil {
		t.Fatalf("the plugin was not registered: %v", err)
	}
	if got.ManifestDigest != want.ManifestDigest || got.ChecksumsDigest != want.SumsDigest {
		t.Fatalf("installed digests %s/%s, want the recorded %s/%s; a reference must name what lands on disk",
			got.ManifestDigest, got.ChecksumsDigest, want.ManifestDigest, want.SumsDigest)
	}
}

func TestInstallExpectedRefusesAReplacedReleaseBeforeInstalling(t *testing.T) {
	for name, mutate := range map[string]func(*usecase.ExpectedInstall){
		"manifest": func(e *usecase.ExpectedInstall) { e.ManifestDigest = hexSHA256([]byte("another manifest")) },
		"sums":     func(e *usecase.ExpectedInstall) { e.SumsDigest = hexSHA256([]byte("another SHA256SUMS")) },
	} {
		t.Run(name, func(t *testing.T) {
			rig := newOfferRig(t)
			want := rig.expected(t)
			mutate(&want)
			err := rig.service.InstallExpected(context.Background(), want)
			if !errors.Is(err, usecase.ErrPluginDigestMismatch) {
				t.Fatalf("err = %v, want ErrPluginDigestMismatch: a release replaced behind the same tag must be refused (spec 6.4)", err)
			}
			if _, err := rig.registry.Get("com.example.offered"); err == nil {
				t.Fatal("a mismatched build was registered; it must not run before the user accepts it")
			}
			if _, err := os.Stat(rig.installedDir()); err == nil {
				t.Fatal("a mismatched build was written to the plugins directory")
			}
		})
	}
}

func TestInstallExpectedInstallsADifferentBuildTheUserAccepted(t *testing.T) {
	rig := newOfferRig(t)
	want := rig.expected(t)
	want.ManifestDigest = hexSHA256([]byte("another manifest"))
	want.AcceptDifferent = true
	if err := rig.service.InstallExpected(context.Background(), want); err != nil {
		t.Fatalf("an accepted different build was refused: %v", err)
	}
	if _, err := rig.registry.Get("com.example.offered"); err != nil {
		t.Fatalf("the accepted build was not registered: %v", err)
	}
}

func TestInstallExpectedRegistersTheRepositoryUntrusted(t *testing.T) {
	rig := newOfferRig(t)
	if err := rig.service.InstallExpected(context.Background(), rig.expected(t)); err != nil {
		t.Fatal(err)
	}
	repo, err := rig.storage.Get(context.Background(), offerRepoURL)
	if err != nil || repo == nil {
		t.Fatalf("the offered repository was not added as a source: %v", err)
	}
	if repo.Trusted {
		t.Fatal("a repository added by an offer was marked trusted; trust is the user's explicit choice")
	}
}

func TestInstallExpectedStillEnforcesConsents(t *testing.T) {
	const sharedManifest = `{
  "id": "com.example.offered",
  "name": "Offered",
  "version": "1.0.0",
  "engine": {"type": "go-binary", "entry": "demo.exe"},
  "isolation": "per-plugin",
  "capabilities": {"session": {"connectProtocols": ["demo"], "allowMultiSession": true}}
}`
	rig := newOfferRigFor(t, sharedManifest)
	want := rig.expected(t)
	if err := rig.service.InstallExpected(context.Background(), want); err == nil {
		t.Fatal("an offer installed without the multi-session consent; an offer must not skip the consent screen")
	}
	if _, err := rig.registry.Get("com.example.offered"); err == nil {
		t.Fatal("a refused install was registered")
	}
	want.Consents.MultiSession = true
	if err := rig.service.InstallExpected(context.Background(), want); err != nil {
		t.Fatalf("install with the consent given: %v", err)
	}
}

func TestInstallAndUninstallTellTheInstalledChangedHandler(t *testing.T) {
	dataRoot := t.TempDir()
	manager := usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry: usecase.NewPluginRegistry(), Host: idleProcessHost{},
		LoadBundle: infraplugin.LoadPluginSource, InstallBundle: infraplugin.InstallFromSource, InstallRoot: dataRoot,
		PortableData: portable.NewDataStore(dataRoot, "", nil),
	})
	calls := 0
	manager.SetInstalledChangedHandler(func() { calls++ })

	src := packBundle(t, offerManifest)
	if _, err := manager.Install(src, domainplugin.InstallTrustPolicy{}, false, false, false); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("handler called %d times after an install; the synced plugin list records it at once (spec 6.4)", calls)
	}
	if err := manager.UninstallPlugin(context.Background(), "com.example.offered", false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("handler called %d times after install and uninstall, want 2", calls)
	}
	if err := manager.UninstallPlugin(context.Background(), "com.example.offered", false); err == nil || calls != 2 {
		t.Fatalf("a failed uninstall: err %v, calls %d; nothing changed, so nothing is recorded", err, calls)
	}
}
