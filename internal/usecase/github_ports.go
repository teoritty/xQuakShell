package usecase

import (
	"context"

	domainplugin "xquakshell/internal/domain/plugin"
)

// GitHubAPIClient abstracts GitHub repository and release access.
type GitHubAPIClient interface {
	GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	GetLatestRelease(ctx context.Context, owner, repo string) (*domainplugin.GitHubRelease, error)
	ListPublishedReleases(ctx context.Context, owner, repo string) ([]domainplugin.GitHubRelease, error)
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*domainplugin.GitHubRelease, error)
}

// PluginBinaryDownloader downloads plugin release assets from GitHub Releases.
// Infra implementations return a cleanup func that removes any temp directories they create;
// the returned asset's Path is only valid until it runs.
type PluginBinaryDownloader interface {
	DownloadAsset(ctx context.Context, req domainplugin.AssetDownloadRequest) (asset domainplugin.DownloadedAsset, cleanup func(), err error)
	DownloadAssetContent(ctx context.Context, owner, repo, tag, assetName string) ([]byte, error)
}

// GitHubPluginStager prepares a local plugin directory from a downloaded release asset.
//
// manifest is the repository's xqsp.json. For a bare binary it is the manifest that gets written
// into the staging directory, because a binary carries none. For a bundle it is only an
// expectation: the bundle's own plugin.json is what lands on disk and what the returned
// StagedPlugin reports, so the caller can check the two agree.
//
// Infra implementations return a cleanup func that removes the staging directory.
type GitHubPluginStager func(asset domainplugin.DownloadedAsset, manifest domainplugin.Manifest) (staged domainplugin.StagedPlugin, cleanup func(), err error)

// GitHubInstallMetaWriter persists install provenance into a staged plugin directory.
type GitHubInstallMetaWriter interface {
	Write(stageDir string, meta domainplugin.PluginInstallMeta) error
}
