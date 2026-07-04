package plugin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infracache "ssh-client/internal/infra/cache"
	infragithub "ssh-client/internal/infra/github"
	infrapersistence "ssh-client/internal/infra/persistence"
	"ssh-client/internal/usecase"
)

type recordingGitHubClient struct {
	manifest []byte
	releases []infragithub.Release
}

func (r *recordingGitHubClient) GetFileContent(_ context.Context, _, _, path, _ string) ([]byte, error) {
	if path == domainplugin.XQSPManifestFile {
		return r.manifest, nil
	}
	return nil, errors.New("file not found: " + path)
}

func (r *recordingGitHubClient) GetLatestRelease(_ context.Context, _, _ string) (*infragithub.Release, error) {
	if len(r.releases) == 0 {
		return nil, domainplugin.ErrNoReleases
	}
	release := r.releases[0]
	return &release, nil
}

func (r *recordingGitHubClient) ListPublishedReleases(_ context.Context, _, _ string) ([]infragithub.Release, error) {
	return r.releases, nil
}

func (r *recordingGitHubClient) GetReleaseByTag(_ context.Context, _, _, tag string) (*infragithub.Release, error) {
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

func (d *recordingDownloader) DownloadBinary(_ context.Context, _, _, tag, _, _ string) (string, error) {
	d.lastTag = tag
	return "", errors.New("download disabled in test")
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

func TestFetchPluginMetadata_ReturnsMultipleReleases(t *testing.T) {
	client := &recordingGitHubClient{
		manifest: []byte(testManifest),
		releases: []infragithub.Release{
			{TagName: "v2.0.0", Assets: []infragithub.Asset{{Name: "demo-windows-amd64.exe"}}},
			{TagName: "v1.0.0", Prerelease: true, Assets: []infragithub.Asset{{Name: "demo-windows-amd64.exe"}}},
		},
	}
	svc := usecase.NewGitHubPluginService(client, nil, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil, nil, t.TempDir())

	meta, err := svc.FetchPluginMetadata(context.Background(), "https://github.com/user/repo")
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
		releases: []infragithub.Release{
			{TagName: "v2.0.0", Assets: []infragithub.Asset{{Name: "demo-windows-amd64.exe"}}},
			{TagName: "v1.0.0", Assets: []infragithub.Asset{{Name: "demo-windows-amd64.exe"}}},
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

	svc := usecase.NewGitHubPluginService(client, downloader, nil, infracache.NewMemoryCache(domainplugin.DefaultCacheTTL), nil, storage, dir)

	err = svc.InstallPluginFromGitHub(ctx, "https://github.com/user/repo", "v1.0.0", false, false, false)
	if err == nil {
		t.Fatal("expected install to fail at download stage")
	}
	if downloader.lastTag != "v1.0.0" {
		t.Fatalf("expected download tag v1.0.0, got %q", downloader.lastTag)
	}
}
