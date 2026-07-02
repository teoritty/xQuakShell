package usecase

import (
	"context"

	domainplugin "ssh-client/internal/domain/plugin"
	infragithub "ssh-client/internal/infra/github"
)

// GitHubAPIClient abstracts GitHub repository and release access.
type GitHubAPIClient interface {
	GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*infragithub.Release, error)
}

// PluginBinaryDownloader downloads plugin binaries from GitHub Releases.
type PluginBinaryDownloader interface {
	DownloadBinary(ctx context.Context, owner, repo, tag, assetName, expectedChecksum string) (string, error)
}

// GitHubPluginStager prepares a local plugin directory from a downloaded binary.
type GitHubPluginStager func(binaryPath string, manifest domainplugin.Manifest) (string, error)
