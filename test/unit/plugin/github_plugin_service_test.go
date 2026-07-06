package plugin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infracache "ssh-client/internal/infra/cache"
	infrapersistence "ssh-client/internal/infra/persistence"
	"ssh-client/internal/usecase"
)

type recordingGitHubClient struct {
	manifest []byte
	releases []domainplugin.GitHubRelease
}

func (r *recordingGitHubClient) GetFileContent(_ context.Context, _, _, path, _ string) ([]byte, error) {
	if path == domainplugin.XQSPManifestFile {
		return r.manifest, nil
	}
	return nil, errors.New("file not found: " + path)
}

func (r *recordingGitHubClient) GetLatestRelease(_ context.Context, _, _ string) (*domainplugin.GitHubRelease, error) {
	if len(r.releases) == 0 {
		return nil, domainplugin.ErrNoReleases
	}
	release := r.releases[0]
	return &release, nil
}

func (r *recordingGitHubClient) ListPublishedReleases(_ context.Context, _, _ string) ([]domainplugin.GitHubRelease, error) {
	return r.releases, nil
}

func (r *recordingGitHubClient) GetReleaseByTag(_ context.Context, _, _, tag string) (*domainplugin.GitHubRelease, error) {
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
}

func (d *recordingDownloader) DownloadBinary(_ context.Context, _, _, tag, _, _ string) (string, func(), error) {
	d.lastTag = tag
	return "", func() {}, errors.New("download disabled in test")
}

func (d *recordingDownloader) DownloadAssetContent(_ context.Context, _, _, tag, _ string) ([]byte, error) {
	d.lastTag = tag
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
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []domainplugin.GitHubRelease{
			{TagName: "v2.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
			{TagName: "v1.0.0", Assets: []domainplugin.GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}}},
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

	err = svc.InstallPluginFromGitHub(ctx, "https://github.com/user/repo", "v1.0.0", false, false, false, false, false)
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
	downloader := &recordingDownloader{}
	svc := newTestGitHubPluginService(t, client, downloader, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil)

	if _, err := svc.FetchPluginMetadataForRelease(context.Background(), "https://github.com/user/repo", "v1.0.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if downloader.lastTag != "v1.0.0" {
		t.Fatalf("expected checksum download for release metadata, got tag %q", downloader.lastTag)
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
