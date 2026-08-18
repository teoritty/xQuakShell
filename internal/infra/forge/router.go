// Package forge routes repository and release requests to the platform that serves them.
package forge

import (
	"context"
	"fmt"
	"io"

	domainplugin "xquakshell/internal/domain/plugin"
	infragithub "xquakshell/internal/infra/github"
	infragitlab "xquakshell/internal/infra/gitlab"
)

// Client is what one forge's adapter offers. Both infra clients satisfy it already; the interface
// exists so the router holds them behind one type and a third forge is a constructor argument.
type Client interface {
	GetFileContent(ctx context.Context, repo domainplugin.RepoRef, path, ref string) ([]byte, error)
	GetLatestRelease(ctx context.Context, repo domainplugin.RepoRef) (*domainplugin.GitHubRelease, error)
	ListPublishedReleases(ctx context.Context, repo domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error)
	GetReleaseByTag(ctx context.Context, repo domainplugin.RepoRef, tag string) (*domainplugin.GitHubRelease, error)
	DownloadAsset(ctx context.Context, downloadURL string) (io.ReadCloser, error)
}

// Router dispatches each call to the forge named by the RepoRef it was given.
//
// It is the only place in the build that decides GitHub versus GitLab. Everything above it works
// in RepoRef, which is why adding GitLab needed no change to how the plugin service is written:
// the ref it already had to parse carries the answer.
type Router struct {
	clients map[domainplugin.Forge]Client
}

// NewRouter builds the router over the forges this build supports.
func NewRouter() *Router {
	return &Router{clients: map[domainplugin.Forge]Client{
		domainplugin.ForgeGitHub: infragithub.NewUseCaseClient(infragithub.NewClient()),
		domainplugin.ForgeGitLab: infragitlab.NewUseCaseClient(infragitlab.NewClient()),
	}}
}

// NewRouterWithClients builds a router over explicit per-forge clients, for tests that point a
// forge at an httptest server.
func NewRouterWithClients(clients map[domainplugin.Forge]Client) *Router {
	return &Router{clients: clients}
}

// ErrUnsupportedForge means the ref named a platform this build has no adapter for. It is returned
// rather than defaulted to GitHub: a silent default would send an unauthenticated request for a
// repository path to a host the user never named.
var ErrUnsupportedForge = fmt.Errorf("unsupported forge")

func (r *Router) clientFor(forge domainplugin.Forge) (Client, error) {
	client, ok := r.clients[forge]
	if !ok || client == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedForge, forge)
	}
	return client, nil
}

// GetFileContent fetches a file from the repository on its own forge.
func (r *Router) GetFileContent(ctx context.Context, repo domainplugin.RepoRef, path, ref string) ([]byte, error) {
	client, err := r.clientFor(repo.Forge)
	if err != nil {
		return nil, err
	}
	return client.GetFileContent(ctx, repo, path, ref)
}

// GetLatestRelease fetches the latest release from the repository's own forge.
func (r *Router) GetLatestRelease(ctx context.Context, repo domainplugin.RepoRef) (*domainplugin.GitHubRelease, error) {
	client, err := r.clientFor(repo.Forge)
	if err != nil {
		return nil, err
	}
	return client.GetLatestRelease(ctx, repo)
}

// ListPublishedReleases lists releases from the repository's own forge.
func (r *Router) ListPublishedReleases(ctx context.Context, repo domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error) {
	client, err := r.clientFor(repo.Forge)
	if err != nil {
		return nil, err
	}
	return client.ListPublishedReleases(ctx, repo)
}

// GetReleaseByTag fetches one release from the repository's own forge.
func (r *Router) GetReleaseByTag(ctx context.Context, repo domainplugin.RepoRef, tag string) (*domainplugin.GitHubRelease, error) {
	client, err := r.clientFor(repo.Forge)
	if err != nil {
		return nil, err
	}
	return client.GetReleaseByTag(ctx, repo, tag)
}

// DownloadAsset fetches an asset URL using the client of the forge that published it.
//
// The forge is passed explicitly because the URL alone does not identify it: both platforms
// redirect asset downloads to CDN hosts that belong to neither.
func (r *Router) DownloadAsset(ctx context.Context, forge domainplugin.Forge, downloadURL string) (io.ReadCloser, error) {
	client, err := r.clientFor(forge)
	if err != nil {
		return nil, err
	}
	return client.DownloadAsset(ctx, downloadURL)
}
