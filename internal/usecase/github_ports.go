package usecase

import (
	"context"

	domainplugin "ssh-client/internal/domain/plugin"
)

// GitHubAPIClient abstracts GitHub repository and release access.
type GitHubAPIClient interface {
	GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*domainplugin.GitHubRelease, error)
	ListPublishedReleases(ctx context.Context, owner, repo string) ([]domainplugin.GitHubRelease, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*domainplugin.GitHubRelease, error)
}

// PluginBinaryDownloader downloads plugin binaries from GitHub Releases.
type PluginBinaryDownloader interface {
	DownloadBinary(ctx context.Context, owner, repo, tag, assetName, expectedChecksum string) (string, error)
}

// GitHubPluginStager prepares a local plugin directory from a downloaded binary.
type GitHubPluginStager func(binaryPath string, manifest domainplugin.Manifest) (string, error)

// GitHubInstallMetaWriter persists install provenance into a staged plugin directory.
type GitHubInstallMetaWriter interface {
	Write(stageDir string, meta domainplugin.PluginInstallMeta) error
}
