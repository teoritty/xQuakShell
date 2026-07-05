package github

import (
	"context"

	domainplugin "ssh-client/internal/domain/plugin"
)

// UseCaseClient adapts the GitHub REST client to usecase ports with domain DTOs.
type UseCaseClient struct {
	inner *Client
}

// NewUseCaseClient wraps a GitHub API client for the usecase layer.
func NewUseCaseClient(inner *Client) *UseCaseClient {
	return &UseCaseClient{inner: inner}
}

// GetFileContent fetches a file from the repository.
func (c *UseCaseClient) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	return c.inner.GetFileContent(ctx, owner, repo, path, ref)
}

// GetLatestRelease fetches the latest release for a repository.
func (c *UseCaseClient) GetLatestRelease(ctx context.Context, owner, repo string) (*domainplugin.GitHubRelease, error) {
	release, err := c.inner.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	domainRelease := ToDomainRelease(*release)
	return &domainRelease, nil
}

// ListPublishedReleases returns published releases, newest first.
func (c *UseCaseClient) ListPublishedReleases(ctx context.Context, owner, repo string) ([]domainplugin.GitHubRelease, error) {
	releases, err := c.inner.ListPublishedReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return ToDomainReleases(releases), nil
}

// GetReleaseByTag fetches a release by tag name.
func (c *UseCaseClient) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*domainplugin.GitHubRelease, error) {
	release, err := c.inner.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, err
	}
	domainRelease := ToDomainRelease(*release)
	return &domainRelease, nil
}
