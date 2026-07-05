package plugin

import (
	"strings"
	"time"
)

// GitHubReleaseAsset names a binary or checksum file in a GitHub release.
type GitHubReleaseAsset struct {
	Name          string
	DownloadCount int
}

// GitHubRelease is a published GitHub release used for plugin discovery and install.
type GitHubRelease struct {
	TagName     string
	Name        string
	PublishedAt string
	Prerelease  bool
	Assets      []GitHubReleaseAsset
}

// ParseGitHubAssetName extracts os/arch from a release asset filename.
func ParseGitHubAssetName(filename string) (osName, arch string) {
	name := strings.ToLower(filename)
	name = strings.TrimSuffix(name, ".exe")
	name = strings.TrimSuffix(name, ".zip")
	name = strings.TrimSuffix(name, ".tar.gz")
	name = strings.TrimSuffix(name, ".tgz")

	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return "", ""
	}

	arch = parts[len(parts)-1]
	osName = parts[len(parts)-2]
	if !IsValidPlatformOS(osName) || !IsValidPlatformArch(arch) {
		return "", ""
	}
	return osName, arch
}

// ExtractPlatformsFromAssets maps release assets to supported platform entries.
func ExtractPlatformsFromAssets(assets []GitHubReleaseAsset, checksums map[string]string) []PlatformInfo {
	var platforms []PlatformInfo
	for _, asset := range assets {
		if asset.Name == "SHA256SUMS" || asset.Name == "checksums.txt" {
			continue
		}
		osName, arch := ParseGitHubAssetName(asset.Name)
		if osName == "" || arch == "" {
			continue
		}
		platforms = append(platforms, PlatformInfo{
			OS:        osName,
			Arch:      arch,
			AssetName: asset.Name,
			Checksum:  checksums[asset.Name],
		})
	}
	return platforms
}

// BuildReleaseSummaries converts GitHub releases into discovery summaries without checksum downloads.
func BuildReleaseSummaries(releases []GitHubRelease) []GitHubReleaseSummary {
	summaries := make([]GitHubReleaseSummary, 0, len(releases))
	for i := range releases {
		platforms := ExtractPlatformsFromAssets(releases[i].Assets, nil)
		summaries = append(summaries, GitHubReleaseSummary{
			Tag:               releases[i].TagName,
			Name:              releases[i].Name,
			PublishedAt:       ParseReleasePublishedAt(releases[i].PublishedAt),
			Prerelease:        releases[i].Prerelease,
			Platforms:         platforms,
			PlatformSupported: ReleaseSummarySupportsCurrentPlatform(platforms),
		})
	}
	return summaries
}

// HasReleaseWithPlatforms reports whether any release exposes platform assets.
func HasReleaseWithPlatforms(releases []GitHubReleaseSummary) bool {
	for _, release := range releases {
		if len(release.Platforms) > 0 {
			return true
		}
	}
	return false
}

// ParseChecksumsFile parses SHA256SUMS content into asset name -> checksum map.
func ParseChecksumsFile(content string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := strings.TrimPrefix(parts[0], "*")
		out[parts[len(parts)-1]] = hash
	}
	return out
}

// ParseReleasePublishedAt normalizes GitHub's published_at timestamp.
func ParseReleasePublishedAt(raw string) string {
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return t.Format(time.RFC3339)
}

// TotalReleaseDownloadCount sums asset download counts for a release.
func TotalReleaseDownloadCount(assets []GitHubReleaseAsset) int {
	total := 0
	for _, a := range assets {
		total += a.DownloadCount
	}
	return total
}
