package usecase

import (
	"context"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

func (s *GitHubPluginService) downloadBinary(
	ctx context.Context,
	owner, repo, tag, assetName, checksum string,
) (path string, cleanup func(), err error) {
	if s.downloader == nil {
		return "", func() {}, fmt.Errorf("plugin downloader unavailable")
	}
	return s.downloader.DownloadBinary(ctx, owner, repo, tag, assetName, checksum)
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
