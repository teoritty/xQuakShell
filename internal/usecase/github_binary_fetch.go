package usecase

import (
	"context"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

func (s *GitHubPluginService) downloadAsset(
	ctx context.Context,
	req domainplugin.AssetDownloadRequest,
) (domainplugin.DownloadedAsset, func(), error) {
	if s.downloader == nil {
		return domainplugin.DownloadedAsset{}, func() {}, fmt.Errorf("plugin downloader unavailable")
	}
	return s.downloader.DownloadAsset(ctx, req)
}

func (s *GitHubPluginService) loadReleaseChecksums(ctx context.Context, owner, repo string, release *domainplugin.GitHubRelease) map[string]string {
	if release == nil || s.downloader == nil {
		return nil
	}
	for _, asset := range release.Assets {
		if asset.Name != "SHA256SUMS" && asset.Name != "checksums.txt" {
			continue
		}
		data, err := s.downloader.DownloadAssetContent(ctx, owner, repo, release.TagName, asset.Name)
		if err != nil {
			continue
		}
		return domainplugin.ParseChecksumsFile(string(data))
	}
	return nil
}
