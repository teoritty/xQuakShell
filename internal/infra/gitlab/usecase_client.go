package gitlab

import (
	"context"
	"io"

	domainplugin "xquakshell/internal/domain/plugin"
)

// UseCaseClient adapts the GitLab REST client to the usecase port with domain DTOs.
type UseCaseClient struct {
	inner *Client
}

// NewUseCaseClient wraps a GitLab API client for the usecase layer.
func NewUseCaseClient(inner *Client) *UseCaseClient {
	return &UseCaseClient{inner: inner}
}

// GetFileContent fetches a file from the repository.
func (c *UseCaseClient) GetFileContent(ctx context.Context, repo domainplugin.RepoRef, path, ref string) ([]byte, error) {
	return c.inner.GetFileContent(ctx, repo.Owner, repo.Repo, path, ref)
}

// GetLatestRelease fetches the latest published release for a repository.
func (c *UseCaseClient) GetLatestRelease(ctx context.Context, repo domainplugin.RepoRef) (*domainplugin.GitHubRelease, error) {
	release, err := c.inner.GetLatestRelease(ctx, repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	domainRelease := ToDomainRelease(*release)
	return &domainRelease, nil
}

// ListPublishedReleases returns published releases, newest first.
func (c *UseCaseClient) ListPublishedReleases(ctx context.Context, repo domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error) {
	releases, err := c.inner.ListPublishedReleases(ctx, repo.Owner, repo.Repo)
	if err != nil {
		return nil, err
	}
	return ToDomainReleases(releases), nil
}

// GetReleaseByTag fetches a release by tag name.
func (c *UseCaseClient) GetReleaseByTag(ctx context.Context, repo domainplugin.RepoRef, tag string) (*domainplugin.GitHubRelease, error) {
	release, err := c.inner.GetReleaseByTag(ctx, repo.Owner, repo.Repo, tag)
	if err != nil {
		return nil, err
	}
	domainRelease := ToDomainRelease(*release)
	return &domainRelease, nil
}

// DownloadAsset downloads a release asset from the given URL.
func (c *UseCaseClient) DownloadAsset(ctx context.Context, downloadURL string) (io.ReadCloser, error) {
	return c.inner.DownloadAsset(ctx, downloadURL)
}
