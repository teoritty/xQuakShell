package usecase

import (
	"context"

	domainplugin "xquakshell/internal/domain/plugin"
)

// GitHubAPIClient abstracts repository and release access on any supported forge.
//
// It takes a RepoRef rather than an owner/repo pair so the forge travels with the request: the
// implementation the composition root supplies is a router, and the ref is the only thing telling
// it whether a call is GitHub's REST shape or GitLab's. The name is unchanged because it is what
// the plugin stack has always called this port.
type GitHubAPIClient interface {
	GetFileContent(ctx context.Context, repo domainplugin.RepoRef, path, ref string) ([]byte, error)
	GetLatestRelease(ctx context.Context, repo domainplugin.RepoRef) (*domainplugin.GitHubRelease, error)
	ListPublishedReleases(ctx context.Context, repo domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error)
	GetReleaseByTag(ctx context.Context, repo domainplugin.RepoRef, tag string) (*domainplugin.GitHubRelease, error)
}

// PluginBinaryDownloader downloads plugin release assets from a forge's releases.
// Infra implementations return a cleanup func that removes any temp directories they create;
// the returned asset's Path is only valid until it runs.
type PluginBinaryDownloader interface {
	DownloadAsset(ctx context.Context, req domainplugin.AssetDownloadRequest) (asset domainplugin.DownloadedAsset, cleanup func(), err error)
	DownloadAssetContent(ctx context.Context, repo domainplugin.RepoRef, tag, assetName string) ([]byte, error)
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
