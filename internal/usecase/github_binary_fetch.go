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

// loadReleaseChecksums fetches the release-level SHA256SUMS, when the release published one.
//
// A nil map and a nil error mean the release carries no checksums asset at all. A failure to
// fetch one that IS listed is an error, and that distinction is the whole point of the signature:
// this used to `continue` past the failure and return nil, which is the same value as "the
// release published none", so a rate limit, a 5xx or a dropped connection silently turned the
// download's integrity check off. Anyone able to fail one HTTPS request - not forge it, just fail
// it - got an unverified install out of it, and nothing anywhere said so.
func (s *GitHubPluginService) loadReleaseChecksums(ctx context.Context, owner, repo string, release *domainplugin.GitHubRelease) (map[string]string, error) {
	if release == nil || s.downloader == nil {
		return nil, nil
	}
	for _, asset := range release.Assets {
		if asset.Name != "SHA256SUMS" && asset.Name != "checksums.txt" {
			continue
		}
		data, err := s.downloader.DownloadAssetContent(ctx, owner, repo, release.TagName, asset.Name)
		if err != nil {
			return nil, fmt.Errorf("release %s publishes %s but it could not be fetched: %w", release.TagName, asset.Name, err)
		}
		return domainplugin.ParseChecksumsFile(string(data)), nil
	}
	return nil, nil
}
