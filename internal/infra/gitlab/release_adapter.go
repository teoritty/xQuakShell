package gitlab

import domainplugin "xquakshell/internal/domain/plugin"

// ToDomainRelease maps a GitLab release to the domain DTO the plugin stack reads.
//
// DownloadCount is left at zero on every asset: GitLab's releases API publishes no per-asset
// counter, and inventing one would put a number in the UI that means nothing.
func ToDomainRelease(r Release) domainplugin.GitHubRelease {
	assets := make([]domainplugin.GitHubReleaseAsset, len(r.Assets.Links))
	for i := range r.Assets.Links {
		assets[i] = domainplugin.GitHubReleaseAsset{
			Name:        r.Assets.Links[i].Name,
			DownloadURL: r.Assets.Links[i].DownloadURL(),
		}
	}
	return domainplugin.GitHubRelease{
		TagName:     r.TagName,
		Name:        r.Name,
		PublishedAt: r.ReleasedAt,
		Prerelease:  r.UpcomingRelease,
		Assets:      assets,
	}
}

// ToDomainReleases maps GitLab releases to domain DTOs.
func ToDomainReleases(releases []Release) []domainplugin.GitHubRelease {
	out := make([]domainplugin.GitHubRelease, len(releases))
	for i := range releases {
		out[i] = ToDomainRelease(releases[i])
	}
	return out
}
