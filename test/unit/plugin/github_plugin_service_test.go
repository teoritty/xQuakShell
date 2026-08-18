package plugin_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infracache "xquakshell/internal/infra/cache"
	infrapersistence "xquakshell/internal/infra/persistence"
	"xquakshell/internal/usecase"
)

type recordingGitHubClient struct {
	manifest []byte
	releases []domainplugin.GitHubRelease
	// seenRefs records every ref the service routed here, so a test can assert the forge survived
	// the trip from repository URL to API call.
	seenRefs []domainplugin.RepoRef
}

func (r *recordingGitHubClient) GetFileContent(_ context.Context, ref domainplugin.RepoRef, path, _ string) ([]byte, error) {
	r.seenRefs = append(r.seenRefs, ref)
	if path == domainplugin.XQSPManifestFile {
		return r.manifest, nil
	}
	return nil, errors.New("file not found: " + path)
}

func (r *recordingGitHubClient) GetLatestRelease(_ context.Context, _ domainplugin.RepoRef) (*domainplugin.GitHubRelease, error) {
	if len(r.releases) == 0 {
		return nil, domainplugin.ErrNoReleases
	}
	release := r.releases[0]
	return &release, nil
}

func (r *recordingGitHubClient) ListPublishedReleases(_ context.Context, _ domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error) {
	return r.releases, nil
}

func (r *recordingGitHubClient) GetReleaseByTag(_ context.Context, _ domainplugin.RepoRef, tag string) (*domainplugin.GitHubRelease, error) {
	for i := range r.releases {
		if r.releases[i].TagName == tag {
			release := r.releases[i]
			return &release, nil
		}
	}
	return nil, domainplugin.ErrReleaseAssetNotFound
}

type recordingDownloader struct {
	lastTag string
	// checksums is served by DownloadAssetContent when set. Without it the fetch fails, and a
	// failed SHA256SUMS fetch now aborts the metadata load instead of being treated as "this
	// release published no checksums" - so a test that wants the load to succeed has to say what
	// the checksums file contains.
	checksums []byte
}

func (d *recordingDownloader) DownloadAsset(_ context.Context, req domainplugin.AssetDownloadRequest) (domainplugin.DownloadedAsset, func(), error) {
	d.lastTag = req.Tag
	return domainplugin.DownloadedAsset{}, func() {}, errors.New("download disabled in test")
}

func (d *recordingDownloader) DownloadAssetContent(_ context.Context, _ domainplugin.RepoRef, tag, _ string) ([]byte, error) {
	d.lastTag = tag
	if d.checksums != nil {
		return d.checksums, nil
	}
	return nil, errors.New("download disabled in test")
}

const testManifest = `{
  "id": "com.example.demo",
  "name": "Demo Plugin",
  "version": "1.0.0",
  "engine": {
    "type": "go-binary",
    "entry": "plugin.exe"
  }
}`

// currentPlatformAssetName builds a release asset name matching runtime.GOOS/GOARCH
// so that GetPlatformForCurrent() resolves it regardless of the CI platform.
func currentPlatformAssetName() string {
	name := "demo-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func newTestGitHubPluginService(t *testing.T, client usecase.GitHubAPIClient, downloader usecase.PluginBinaryDownloader, cache domainplugin.GitHubCache, storage domainplugin.GitHubRepositoryStorage) *usecase.GitHubPluginService {
	t.Helper()
	return usecase.NewGitHubPluginService(client, downloader, nil, nil, cache, nil, storage)
}

func TestFetchPluginMetadata_ReturnsMultipleReleases(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v2.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
			{TagName: "v1.0.0", Prerelease: true, Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
		},
	}
	svc := newTestGitHubPluginService(t, client, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

	meta, err := svc.FetchPluginMetadata(context.Background(), "https://github.com/user/repo", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta.AvailableReleases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(meta.AvailableReleases))
	}
	if meta.AvailableReleases[0].Tag != "v2.0.0" {
		t.Fatalf("unexpected first tag: %s", meta.AvailableReleases[0].Tag)
	}
}

func TestInstallPluginFromGitHub_UsesSelectedReleaseTag(t *testing.T) {
	// The asset must match the platform the test runs on: install resolves
	// GetPlatformForCurrent() before invoking the downloader, so a windows-only
	// asset list makes the install fail with ErrPlatformNotSupported on Linux CI
	// and the downloader is never called.
	asset := currentPlatformAssetName()
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v2.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: asset}}},
			{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: asset}}},
		},
	}
	downloader := &recordingDownloader{}
	dir := t.TempDir()
	storage, err := infrapersistence.NewFileGitHubRepositoryStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := storage.Add(ctx, domainplugin.GitHubRepository{
		URL:         "https://github.com/user/repo",
		Owner:       "user",
		Repo:        "repo",
		DisplayName: "user/repo",
		AddedAt:     time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	svc := usecase.NewGitHubPluginService(client, downloader, nil, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil, storage)

	err = svc.InstallPluginFromGitHub(ctx, "https://github.com/user/repo", "v1.0.0", false, false, false, false, false, false)
	if err == nil {
		t.Fatal("expected install to fail at download stage")
	}
	if downloader.lastTag != "v1.0.0" {
		t.Fatalf("expected download tag v1.0.0, got %q", downloader.lastTag)
	}
}

func TestFetchPluginMetadata_ForceRefreshBypassesCache(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
		},
	}
	cache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	svc := newTestGitHubPluginService(t, client, nil, cache, nil)
	ctx := context.Background()

	first, err := svc.FetchPluginMetadata(ctx, "https://github.com/user/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.FetchPluginMetadata(ctx, "https://github.com/user/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("expected cached metadata pointer reuse")
	}

	client.releases[0].TagName = "v2.0.0"
	third, err := svc.FetchPluginMetadata(ctx, "https://github.com/user/repo", true)
	if err != nil {
		t.Fatal(err)
	}
	if third.LatestRelease != "v2.0.0" {
		t.Fatalf("expected refreshed latest release v2.0.0, got %q", third.LatestRelease)
	}
}

func TestValidateReleaseTag_RejectsUnknownTag(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
		},
	}
	svc := newTestGitHubPluginService(t, client, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)
	_, err := svc.PreviewInstall(context.Background(), "https://github.com/user/repo", "v9.9.9")
	if err == nil {
		t.Fatal("expected invalid release tag error")
	}
	if !errors.Is(err, domainplugin.ErrInvalidReleaseTag) {
		t.Fatalf("expected ErrInvalidReleaseTag, got %v", err)
	}
}

func TestFetchPluginMetadata_ListViewSkipsChecksumDownloads(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{
				TagName: "v1.0.0",
				Assets: []domainplugin.GitHubReleaseAsset{
					{Name: "SHA256SUMS"},
					{Name: "demo-windows-amd64.exe"},
				},
			},
		},
	}
	downloader := &recordingDownloader{}
	svc := newTestGitHubPluginService(t, client, downloader, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

	if _, err := svc.FetchPluginMetadata(context.Background(), "https://github.com/user/repo", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if downloader.lastTag != "" {
		t.Fatalf("list metadata should not download release assets, got tag %q", downloader.lastTag)
	}
}

func TestFetchPluginMetadataForRelease_LoadsChecksumsForInstall(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{
				TagName: "v1.0.0",
				Assets: []domainplugin.GitHubReleaseAsset{
					{Name: "SHA256SUMS"},
					{Name: "demo-windows-amd64.exe"},
				},
			},
		},
	}
	downloader := &recordingDownloader{
		checksums: []byte("0123456789abcdef  demo-windows-amd64.exe\n"),
	}
	svc := newTestGitHubPluginService(t, client, downloader, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

	if _, err := svc.FetchPluginMetadataForRelease(context.Background(), "https://github.com/user/repo", "v1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if downloader.lastTag != "v1.0.0" {
		t.Fatalf("expected checksum download for release metadata, got tag %q", downloader.lastTag)
	}
}

// The release lists SHA256SUMS and fetching it fails. That must stop the install, not fall through
// to "this release published no checksums" - which is what it used to do, silently turning the
// download's only integrity check off for anyone able to fail a single HTTPS request.
func TestFetchPluginMetadataForRelease_ChecksumFetchFailureAbortsTheLoad(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{
				TagName: "v1.0.0",
				Assets: []domainplugin.GitHubReleaseAsset{
					{Name: "SHA256SUMS"},
					{Name: "demo-windows-amd64.exe"},
				},
			},
		},
	}
	downloader := &recordingDownloader{}
	svc := newTestGitHubPluginService(t, client, downloader, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

	if _, err := svc.FetchPluginMetadataForRelease(context.Background(), "https://github.com/user/repo", "v1.0.0"); err == nil {
		t.Fatal("a failed SHA256SUMS fetch was accepted; the install would proceed unverified")
	}
}

func TestInvalidateMetadataCache_ClearsRepoAndTagEntries(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
		},
	}
	cache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	svc := newTestGitHubPluginService(t, client, nil, cache, nil)
	ctx := context.Background()

	if _, err := svc.FetchPluginMetadata(ctx, "https://github.com/user/repo", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FetchPluginMetadataForRelease(ctx, "https://github.com/user/repo", "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := svc.InvalidateMetadataCache(ctx, "https://github.com/user/repo", ""); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := cache.Get(ctx, "metadata:https://github.com/user/repo"); found {
		t.Fatal("expected list metadata cache cleared")
	}
	if _, found, _ := cache.Get(ctx, "metadata:https://github.com/user/repo:v1.0.0"); found {
		t.Fatal("expected tag metadata cache cleared")
	}
}

// The service resolves a repository URL to a RepoRef and hands it to the client; the forge in that
// ref is the only thing telling the router which platform to speak to. If it is lost anywhere along
// the way a GitLab repository is queried against api.github.com, which answers 404 rather than
// failing loudly, so the user is told their repository does not exist.
func TestFetchPluginMetadata_CarriesTheForgeToTheClient(t *testing.T) {
	cases := []struct {
		repoURL string
		want    domainplugin.Forge
		owner   string
		repo    string
	}{
		{"https://github.com/user/repo", domainplugin.ForgeGitHub, "user", "repo"},
		{"https://gitlab.com/user/repo", domainplugin.ForgeGitLab, "user", "repo"},
		{"https://gitlab.com/group/subgroup/repo", domainplugin.ForgeGitLab, "group/subgroup", "repo"},
	}

	for _, tc := range cases {
		t.Run(tc.repoURL, func(t *testing.T) {
			client := &recordingGitHubClient{
				manifest: []byte(testManifest),
				releases: []domainplugin.GitHubRelease{
					{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: currentPlatformAssetName()}}},
				},
			}
			svc := newTestGitHubPluginService(t, client, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

			if _, err := svc.FetchPluginMetadata(context.Background(), tc.repoURL, false); err != nil {
				t.Fatalf("FetchPluginMetadata err = %v, want nil", err)
			}

			if len(client.seenRefs) == 0 {
				t.Fatal("the client was never called")
			}
			got := client.seenRefs[0]
			if got.Forge != tc.want {
				t.Errorf("forge = %q, want %q", got.Forge, tc.want)
			}
			if got.Owner != tc.owner || got.Repo != tc.repo {
				t.Errorf("owner/repo = %q/%q, want %q/%q", got.Owner, got.Repo, tc.owner, tc.repo)
			}
		})
	}
}
