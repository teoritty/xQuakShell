package usecase_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

type refWorld struct {
	t         *testing.T
	vault     *syncTestVault
	mu        sync.Mutex
	plugins   []domainplugin.InstalledPlugin
	disabled  []string
	enabled   []string
	installer *fakeOfferInstaller
	svc       *usecase.PluginReferenceSync
}

// fakeOfferInstaller stands in for the GitHub pipeline: a successful install puts a plugin with
// the build the release really carries (served, not the expectation) into the world.
type fakeOfferInstaller struct {
	w        *refWorld
	served   string // the manifest digest the "release" carries
	err      error
	requests []usecase.ExpectedInstall
	previews []string
}

func (f *fakeOfferInstaller) PreviewInstall(_ context.Context, repoURL, tag string) (usecase.GitHubPluginPreviewDTO, error) {
	f.previews = append(f.previews, repoURL+"@"+tag)
	return usecase.GitHubPluginPreviewDTO{RepositoryURL: repoURL, ReleaseTag: tag}, f.err
}

func (f *fakeOfferInstaller) InstallExpected(_ context.Context, req usecase.ExpectedInstall) error {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return f.err
	}
	if f.served != req.ManifestDigest && !req.AcceptDifferent {
		return usecase.ErrPluginDigestMismatch
	}
	p := githubInstalled("com.example.echo", strings.TrimPrefix(req.Tag, "v"), f.served)
	p.InstallMeta.RepositoryURL, p.InstallMeta.ReleaseTag = req.RepoURL, req.Tag
	f.w.install(p)
	return nil
}

func newRefWorld(t *testing.T) *refWorld {
	t.Helper()
	d := domain.NewVaultData()
	d.Sync = &domain.VaultSync{Account: &domain.SyncAccount{Username: "u"}}
	w := &refWorld{t: t, vault: &syncTestVault{data: d}}
	w.installer = &fakeOfferInstaller{w: w, served: digestA}
	w.svc = usecase.NewPluginReferenceSync(usecase.PluginReferenceDeps{
		Installer: w.installer,
		Enable:    func(_ context.Context, id string) error { w.enabled = append(w.enabled, id); return nil },
		Vault:     w.vault,
		Installed: func() []domainplugin.InstalledPlugin {
			w.mu.Lock()
			defer w.mu.Unlock()
			return slices.Clone(w.plugins)
		},
		Disable: func(_ context.Context, id string) error { w.disabled = append(w.disabled, id); return nil },
	})
	return w
}

func githubInstalled(id, version, manifestDigest string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{ID: id, Version: version}, Source: domainplugin.SourceUser,
		InstallMeta: &domainplugin.PluginInstallMeta{Source: domainplugin.InstallMetaSourceGitHub,
			RepositoryURL: "https://github.com/example/" + id, ReleaseTag: "v" + version},
		ManifestDigest: manifestDigest, ChecksumsDigest: strings.Repeat("cd", 32)}
}

func (w *refWorld) install(p domainplugin.InstalledPlugin) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.plugins = slices.DeleteFunc(w.plugins, func(x domainplugin.InstalledPlugin) bool { return x.Manifest.ID == p.Manifest.ID })
	w.plugins = append(w.plugins, p)
}

func (w *refWorld) uninstall(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.plugins = slices.DeleteFunc(w.plugins, func(x domainplugin.InstalledPlugin) bool { return x.Manifest.ID == id })
}

func (w *refWorld) record() {
	w.t.Helper()
	if err := w.svc.Record(syncCtx); err != nil {
		w.t.Fatal(err)
	}
}

func (w *refWorld) records() map[string]domain.SyncPlugin {
	d, _ := w.vault.GetData()
	out := map[string]domain.SyncPlugin{}
	for _, p := range d.Sync.Plugins {
		out[p.ID] = p
	}
	return out
}

// arrive puts a record in the vault as a pull from another device would.
func (w *refWorld) arrive(p domain.SyncPlugin) {
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Sync.Plugins = slices.DeleteFunc(d.Sync.Plugins, func(x domain.SyncPlugin) bool { return x.ID == p.ID })
		d.Sync.Plugins = append(d.Sync.Plugins, p)
		return nil
	})
}

func (w *refWorld) removeRecord(id string) {
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error {
		d.Sync.Plugins = slices.DeleteFunc(d.Sync.Plugins, func(x domain.SyncPlugin) bool { return x.ID == id })
		return nil
	})
}

func (w *refWorld) offers() map[string]usecase.PluginOffer {
	w.t.Helper()
	list, err := w.svc.Offers(syncCtx)
	if err != nil {
		w.t.Fatal(err)
	}
	out := map[string]usecase.PluginOffer{}
	for _, o := range list {
		out[o.Plugin.ID] = o
	}
	return out
}

var digestA, digestB = strings.Repeat("aa", 32), strings.Repeat("bb", 32)

func TestAnInstalledPluginIsRecordedByReference(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.install(domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{ID: "com.example.bundled"}, Source: domainplugin.SourceBundled,
		ManifestDigest: digestA})
	w.record()
	got := w.records()
	want := domain.SyncPlugin{ID: "com.example.echo", Version: "1.0.0",
		Source:         domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/com.example.echo", Tag: "v1.0.0"},
		ManifestDigest: digestA, SumsDigest: strings.Repeat("cd", 32)}
	if len(got) != 1 || got["com.example.echo"] != want {
		t.Fatalf("records = %+v; the user's GitHub plugin by reference, never a bundled one", got)
	}
	w.install(githubInstalled("com.example.echo", "1.1.0", digestB))
	w.record()
	if r := w.records()["com.example.echo"]; r.Version != "1.1.0" || r.ManifestDigest != digestB {
		t.Fatalf("record after a local upgrade = %+v", r)
	}
}

func TestAPluginWithoutAGitHubOriginIsManual(t *testing.T) {
	w := newRefWorld(t)
	p := githubInstalled("com.example.local", "0.1.0", digestA)
	p.InstallMeta = nil
	w.install(p)
	w.record()
	if r := w.records()["com.example.local"]; r.Source != (domain.SyncPluginSource{Kind: domain.PluginSourceManual}) {
		t.Fatalf("record = %+v; a plugin from a file is offered for a manual install", r)
	}
}

func TestAPluginFromAnotherDeviceIsOfferedNeverInstalled(t *testing.T) {
	w := newRefWorld(t)
	rec := domain.SyncPlugin{ID: "com.example.echo", Version: "1.0.0", ManifestDigest: digestA, SumsDigest: digestA,
		Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/echo", Tag: "v1.0.0"}}
	w.arrive(rec)
	w.record()
	if _, ok := w.records()["com.example.echo"]; !ok {
		t.Fatal("a record of a plugin this device lacks was deleted")
	}
	o, ok := w.offers()["com.example.echo"]
	if !ok || o.State != usecase.PluginOfferInstall || o.Plugin != rec {
		t.Fatalf("offers = %+v; the plugin is offered for install", w.offers())
	}
	manual := rec
	manual.ID, manual.Source = "com.example.local", domain.SyncPluginSource{Kind: domain.PluginSourceManual}
	w.arrive(manual)
	if o := w.offers()["com.example.local"]; o.State != usecase.PluginOfferManual {
		t.Fatalf("a manual plugin's offer = %+v, want the install-manually state", o)
	}
}

func TestAnOfferInstallIsVerifiedAgainstTheRecord(t *testing.T) {
	w := newRefWorld(t)
	rec := domain.SyncPlugin{ID: "com.example.echo", Version: "1.0.0", ManifestDigest: digestA, SumsDigest: strings.Repeat("cd", 32),
		Source: domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/com.example.echo", Tag: "v1.0.0"}}
	w.arrive(rec)

	w.install(githubInstalled("com.example.echo", "1.0.0", digestB))
	if err := w.svc.VerifyInstalled(syncCtx, "com.example.echo"); !errors.Is(err, usecase.ErrPluginDigestMismatch) {
		t.Fatalf("a replaced release behind the same tag: %v, want ErrPluginDigestMismatch", err)
	}
	w.record()
	if w.records()["com.example.echo"] != rec {
		t.Fatal("an unaccepted mismatching install overwrote the record on every device")
	}
	if o := w.offers()["com.example.echo"]; o.State != usecase.PluginOfferMismatch {
		t.Fatalf("offer = %+v; the mismatch stays visible until the user decides", o)
	}
	if err := w.svc.AcceptInstalled(syncCtx, "com.example.echo"); err != nil {
		t.Fatal(err)
	}
	if r := w.records()["com.example.echo"]; r.ManifestDigest != digestB {
		t.Fatalf("record after accepting = %+v; the accepted build is the new version", r)
	}

	w2 := newRefWorld(t)
	w2.arrive(rec)
	w2.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	if err := w2.svc.VerifyInstalled(syncCtx, "com.example.echo"); err != nil {
		t.Fatalf("a matching install: %v", err)
	}
	if _, ok := w2.offers()["com.example.echo"]; ok {
		t.Fatal("a verified install is still offered")
	}
}

func TestUninstallingHereDeletesTheRecord(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	w.uninstall("com.example.echo")
	w.record()
	if len(w.records()) != 0 {
		t.Fatal("a plugin uninstalled here is still offered on the other devices")
	}
}

func TestRemovedElsewhereDisablesAndOffersUninstall(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	w.removeRecord("com.example.echo")
	w.record()
	if !slices.Equal(w.disabled, []string{"com.example.echo"}) {
		t.Fatalf("disabled = %v; a plugin removed on another device is disabled here", w.disabled)
	}
	if len(w.records()) != 0 {
		t.Fatal("the removed plugin was recorded again from this device")
	}
	if o := w.offers()["com.example.echo"]; o.State != usecase.PluginOfferRemoved {
		t.Fatalf("offer = %+v; uninstall is offered, never automatic", o)
	}
	w.record()
	if len(w.disabled) != 1 {
		t.Fatalf("disabled %d times; once is enough", len(w.disabled))
	}
	if err := w.svc.AcceptInstalled(syncCtx, "com.example.echo"); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.records()["com.example.echo"]; !ok {
		t.Fatal("keeping the plugin did not record it again")
	}
}

func TestNoRecordsWithoutASyncAccount(t *testing.T) {
	w := newRefWorld(t)
	_ = w.vault.UpdateData(syncCtx, func(d *domain.VaultData) error { d.Sync.Account = nil; return nil })
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	if len(w.records()) != 0 {
		t.Fatal("plugins were recorded in a vault that does not sync")
	}
}

func TestAnUpgradeElsewhereIsOfferedNotReverted(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	newer := w.records()["com.example.echo"]
	newer.Version, newer.ManifestDigest, newer.Source.Tag = "2.0.0", digestB, "v2.0.0"
	w.arrive(newer)
	w.record()
	if w.records()["com.example.echo"] != newer {
		t.Fatal("this device wrote its older build back over an upgrade made on another device")
	}
	if o := w.offers()["com.example.echo"]; o.State != usecase.PluginOfferUpdate || o.Plugin != newer {
		t.Fatalf("offer = %+v; the upgrade is offered here", o)
	}
	w.install(githubInstalled("com.example.echo", "1.5.0", digestA+"0"))
	w.record()
	if r := w.records()["com.example.echo"]; r.Version != "1.5.0" {
		t.Fatalf("record = %+v; a local change of the build is still recorded", r)
	}
}

func TestANestedGitHubPathIsNotARepository(t *testing.T) {
	w := newRefWorld(t)
	p := githubInstalled("com.example.echo", "1.0.0", digestA)
	p.InstallMeta.RepositoryURL = "https://github.com/example/echo/tree/main"
	w.install(p)
	w.record()
	if r := w.records()["com.example.echo"]; r.Source.Kind != domain.PluginSourceManual {
		t.Fatalf("record = %+v; a URL that is not owner/name is never offered as a GitHub install", r)
	}
}

func echoRecord(tag, digest string) domain.SyncPlugin {
	return domain.SyncPlugin{ID: "com.example.echo", Version: strings.TrimPrefix(tag, "v"), ManifestDigest: digest,
		SumsDigest: strings.Repeat("cd", 32),
		Source:     domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/com.example.echo", Tag: tag}}
}

func TestInstallingAnOfferRunsThePipelineHeldToTheRecord(t *testing.T) {
	w := newRefWorld(t)
	w.arrive(echoRecord("v1.0.0", digestA))
	consents := domainplugin.InstallConsents{Exec: true}
	if err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.echo", Consents: consents}); err != nil {
		t.Fatal(err)
	}
	want := usecase.ExpectedInstall{RepoURL: "https://github.com/example/com.example.echo", Tag: "v1.0.0",
		ManifestDigest: digestA, SumsDigest: strings.Repeat("cd", 32), Consents: consents}
	if len(w.installer.requests) != 1 || w.installer.requests[0] != want {
		t.Fatalf("pipeline requests = %+v, want exactly %+v: the source and digests of the record, the user's consents", w.installer.requests, want)
	}
	if _, ok := w.offers()["com.example.echo"]; ok {
		t.Fatal("a plugin installed from its offer is still offered")
	}
	if d, _ := w.vault.GetData(); d.Sync.PluginsHere["com.example.echo"] == "" {
		t.Fatal("the installed build was not remembered as this device's; the next scan would misread it as unverified")
	}
}

func TestAReplacedReleaseIsRefusedUnlessAccepted(t *testing.T) {
	w := newRefWorld(t)
	rec := echoRecord("v1.0.0", digestA)
	w.arrive(rec)
	w.installer.served = digestB
	err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.echo"})
	if !errors.Is(err, usecase.ErrPluginDigestMismatch) {
		t.Fatalf("err = %v, want ErrPluginDigestMismatch", err)
	}
	if w.records()["com.example.echo"] != rec {
		t.Fatal("a refused install changed the record")
	}
	if err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.echo", AcceptDifferent: true}); err != nil {
		t.Fatal(err)
	}
	if r := w.records()["com.example.echo"]; r.ManifestDigest != digestB {
		t.Fatalf("record = %+v; an accepted build is the new version on every device", r)
	}
}

func TestAnUpdateOfferInstallsTheRecordedTag(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	w.arrive(echoRecord("v2.0.0", digestB))
	w.installer.served = digestB
	if err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.echo"}); err != nil {
		t.Fatal(err)
	}
	if got := w.installer.requests[0].Tag; got != "v2.0.0" {
		t.Fatalf("installed tag %q, want the upgrade's v2.0.0", got)
	}
	if _, ok := w.offers()["com.example.echo"]; ok {
		t.Fatal("the update is still offered after installing it")
	}
}

func TestAManualOrUnknownOfferHasNoInstall(t *testing.T) {
	w := newRefWorld(t)
	manual := echoRecord("v1.0.0", digestA)
	manual.Source = domain.SyncPluginSource{Kind: domain.PluginSourceManual}
	w.arrive(manual)
	if err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.echo"}); !errors.Is(err, usecase.ErrPluginOfferManual) {
		t.Fatalf("manual offer: %v, want ErrPluginOfferManual", err)
	}
	if _, err := w.svc.PreviewOffer(syncCtx, "com.example.echo"); !errors.Is(err, usecase.ErrPluginOfferManual) {
		t.Fatalf("manual preview: %v, want ErrPluginOfferManual", err)
	}
	if err := w.svc.InstallOffer(syncCtx, usecase.OfferInstallRequest{PluginID: "com.example.other"}); !errors.Is(err, usecase.ErrPluginOfferNotFound) {
		t.Fatalf("unknown offer: %v, want ErrPluginOfferNotFound", err)
	}
	if len(w.installer.requests)+len(w.installer.previews) != 0 {
		t.Fatal("the pipeline was reached for an offer that has no GitHub source")
	}
}

func TestAnOfferPreviewNamesTheRecordedRelease(t *testing.T) {
	w := newRefWorld(t)
	w.arrive(echoRecord("v1.2.0", digestA))
	if _, err := w.svc.PreviewOffer(syncCtx, "com.example.echo"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.installer.previews, []string{"https://github.com/example/com.example.echo@v1.2.0"}) {
		t.Fatalf("previews = %v; the consent screen shows the recorded release", w.installer.previews)
	}
}

func TestKeepingAPluginRemovedElsewhereEnablesItAgain(t *testing.T) {
	w := newRefWorld(t)
	w.install(githubInstalled("com.example.echo", "1.0.0", digestA))
	w.record()
	w.removeRecord("com.example.echo")
	w.record()
	if err := w.svc.KeepPlugin(syncCtx, "com.example.echo"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.enabled, []string{"com.example.echo"}) {
		t.Fatalf("enabled = %v; keeping a plugin another device removed turns it back on", w.enabled)
	}
	if _, ok := w.records()["com.example.echo"]; !ok {
		t.Fatal("keeping did not record the plugin again")
	}

	m := newRefWorld(t)
	m.arrive(echoRecord("v1.0.0", digestA))
	m.install(githubInstalled("com.example.echo", "1.0.0", digestB))
	if err := m.svc.KeepPlugin(syncCtx, "com.example.echo"); err != nil {
		t.Fatal(err)
	}
	if len(m.enabled) != 0 {
		t.Fatalf("enabled = %v; keeping a mismatching build does not touch whether it runs", m.enabled)
	}
	if r := m.records()["com.example.echo"]; r.ManifestDigest != digestB {
		t.Fatalf("record = %+v; the kept build is the new version", r)
	}
}
