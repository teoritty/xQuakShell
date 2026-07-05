package github

import domainplugin "ssh-client/internal/domain/plugin"

// ToDomainRelease maps an infra release to the domain DTO.
func ToDomainRelease(r Release) domainplugin.GitHubRelease {
	assets := make([]domainplugin.GitHubReleaseAsset, len(r.Assets))
	for i := range r.Assets {
		assets[i] = domainplugin.GitHubReleaseAsset{
			Name:          r.Assets[i].Name,
			DownloadCount: r.Assets[i].DownloadCount,
		}
	}
	return domainplugin.GitHubRelease{
		TagName:     r.TagName,
		Name:        r.Name,
		PublishedAt: r.PublishedAt,
		Prerelease:  r.Prerelease,
		Assets:      assets,
	}
}

// ToDomainReleases maps infra releases to domain DTOs.
func ToDomainReleases(releases []Release) []domainplugin.GitHubRelease {
	out := make([]domainplugin.GitHubRelease, len(releases))
	for i := range releases {
		out[i] = ToDomainRelease(releases[i])
	}
	return out
}
