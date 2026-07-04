package wails

import (
	"context"
	"errors"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infracache "ssh-client/internal/infra/cache"
	infragithub "ssh-client/internal/infra/github"
	infrapersistence "ssh-client/internal/infra/persistence"
	"ssh-client/internal/usecase"
)

type stubGitHubAPIClient struct {
	manifest   []byte
	releases   []infragithub.Release
	releaseErr error
}

func (s *stubGitHubAPIClient) GetFileContent(_ context.Context, _, _, path, _ string) ([]byte, error) {
	if path == domainplugin.XQSPManifestFile {
		if len(s.manifest) == 0 {
			return nil, errors.New("file not found: xqsp.json")
		}
		return s.manifest, nil
	}
	return nil, errors.New("file not found: " + path)
}

func (s *stubGitHubAPIClient) GetLatestRelease(_ context.Context, _, _ string) (*infragithub.Release, error) {
	releases, err := s.ListPublishedReleases(context.Background(), "", "")
	if err != nil || len(releases) == 0 {
		return nil, s.releaseErr
	}
	release := releases[0]
	return &release, nil
}

func (s *stubGitHubAPIClient) ListPublishedReleases(_ context.Context, _, _ string) ([]infragithub.Release, error) {
	if s.releaseErr != nil {
		return nil, s.releaseErr
	}
	return s.releases, nil
}

func (s *stubGitHubAPIClient) GetReleaseByTag(_ context.Context, _, _, tag string) (*infragithub.Release, error) {
	for i := range s.releases {
		if s.releases[i].TagName == tag {
			release := s.releases[i]
			return &release, nil
		}
	}
	return nil, domainplugin.ErrReleaseAssetNotFound
}

const testXQSPManifest = `{
  "id": "com.example.demo",
  "name": "Demo Plugin",
  "version": "1.0.0",
  "engine": {
    "type": "go-binary",
    "entry": "plugin.exe"
  }
}`

func newGitHubAddTestAPI(t *testing.T, client usecase.GitHubAPIClient) *AppAPI {
	t.Helper()

	dir := t.TempDir()
	storage, err := infrapersistence.NewFileGitHubRepositoryStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	cache := infracache.NewMemoryCache(domainplugin.DefaultCacheTTL)
	repoService := usecase.NewGitHubRepositoryService(storage, cache)
	pluginService := usecase.NewGitHubPluginService(
		client,
		nil,
		nil,
		cache,
		nil,
		storage,
		dir,
	)

	return &AppAPI{
		githubRepoService:   repoService,
		githubPluginService: pluginService,
	}
}

func TestAddGitHubRepository_ValidationFailureDoesNotPersist(t *testing.T) {
	api := newGitHubAddTestAPI(t, &stubGitHubAPIClient{
		manifest:   []byte(testXQSPManifest),
		releaseErr: domainplugin.ErrNoReleases,
	})

	err := api.AddGitHubRepository(AddGitHubRepositoryRequest{
		URL:     "https://github.com/user/repo",
		Trusted: false,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, domainplugin.ErrNoReleases) {
		t.Fatalf("expected ErrNoReleases, got %v", err)
	}

	repos, err := api.ListGitHubRepositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected no persisted repositories, got %d", len(repos))
	}
}

func TestAddGitHubRepository_SuccessPersistsRepository(t *testing.T) {
	api := newGitHubAddTestAPI(t, &stubGitHubAPIClient{
		manifest: []byte(testXQSPManifest),
		releases: []infragithub.Release{{
			TagName: "v1.0.0",
			Assets: []infragithub.Asset{
				{Name: "demo-windows-amd64.exe"},
			},
		}},
	})

	err := api.AddGitHubRepository(AddGitHubRepositoryRequest{
		URL:     "https://github.com/user/repo",
		Trusted: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repos, err := api.ListGitHubRepositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected one repository, got %d", len(repos))
	}
	if repos[0].URL != "https://github.com/user/repo" || !repos[0].Trusted {
		t.Fatalf("unexpected repository: %+v", repos[0])
	}
}
