package usecase

import (
	"context"
	"fmt"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// FetchPluginMetadata retrieves plugin metadata from a GitHub repository.
func (s *GitHubPluginService) FetchPluginMetadata(ctx context.Context, repoURL string, forceRefresh bool) (*domainplugin.GitHubPluginMetadata, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return nil, err
	}

	cacheKey := metadataCacheKey(normalizedURL, "")
	if forceRefresh {
		_ = s.InvalidateMetadataCache(ctx, normalizedURL, "")
	} else if cached, ok := s.getCachedMetadata(ctx, cacheKey); ok {
		return cached, nil
	}

	owner, repo, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return nil, err
	}

	manifestContent, err := s.apiClient.GetFileContent(ctx, owner, repo, domainplugin.XQSPManifestFile, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, domainplugin.ErrPluginManifestNotFound
		}
		return nil, err
	}

	xqsp, err := domainplugin.ParseXQSPManifest(manifestContent)
	if err != nil {
		return nil, err
	}

	readmeContent, _ := s.apiClient.GetFileContent(ctx, owner, repo, "README.md", "")

	releases, err := s.apiClient.ListPublishedReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%w in %s/%s", domainplugin.ErrNoReleases, owner, repo)
	}

	availableReleases := domainplugin.BuildReleaseSummaries(releases)
	if !domainplugin.HasReleaseWithPlatforms(availableReleases) {
		return nil, fmt.Errorf("%w: no release assets match supported platform naming", domainplugin.ErrInvalidPluginMetadata)
	}

	latest := releases[0]
	latestSummary := availableReleases[0]

	metadata := &domainplugin.GitHubPluginMetadata{
		RepositoryURL:     normalizedURL,
		ID:                xqsp.ID,
		Name:              xqsp.Name,
		Version:           xqsp.Version,
		Description:       xqsp.Description,
		Author:            xqsp.Author,
		Homepage:          xqsp.Homepage,
		License:           xqsp.License,
		MinCoreVersion:    xqsp.MinCoreVersion,
		Platforms:         latestSummary.Platforms,
		AvailableReleases: availableReleases,
		Tags:              xqsp.Tags,
		README:            string(readmeContent),
		LatestRelease:     latest.TagName,
		Prerelease:        latest.Prerelease,
		PublishedAt:       domainplugin.ParseReleasePublishedAt(latest.PublishedAt),
		DownloadCount:     domainplugin.TotalReleaseDownloadCount(latest.Assets),
		Manifest:          xqsp.Manifest,
	}

	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	s.setCachedMetadata(ctx, cacheKey, metadata)
	return metadata, nil
}

// FetchPluginMetadataForRelease retrieves metadata for a specific release tag.
func (s *GitHubPluginService) FetchPluginMetadataForRelease(ctx context.Context, repoURL, releaseTag string) (*domainplugin.GitHubPluginMetadata, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return nil, err
	}
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag == "" {
		return s.FetchPluginMetadata(ctx, normalizedURL, false)
	}

	if err := s.validateReleaseTag(ctx, normalizedURL, releaseTag); err != nil {
		return nil, err
	}

	cacheKey := metadataCacheKey(normalizedURL, releaseTag)
	if cached, ok := s.getCachedMetadata(ctx, cacheKey); ok {
		return cached, nil
	}

	owner, repo, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return nil, err
	}

	release, err := s.apiClient.GetReleaseByTag(ctx, owner, repo, releaseTag)
	if err != nil {
		return nil, err
	}

	manifestContent, err := s.fetchManifestForRelease(ctx, owner, repo, releaseTag)
	if err != nil {
		return nil, err
	}

	xqsp, err := domainplugin.ParseXQSPManifest(manifestContent)
	if err != nil {
		return nil, err
	}

	readmeContent := s.fetchReadmeForRelease(ctx, owner, repo, releaseTag)
	checksums := s.loadReleaseChecksums(ctx, owner, repo, release)
	platforms := domainplugin.ExtractPlatformsFromAssets(release.Assets, checksums)
	if len(platforms) == 0 {
		return nil, fmt.Errorf("%w: no release assets match supported platform naming", domainplugin.ErrInvalidPluginMetadata)
	}

	metadata := &domainplugin.GitHubPluginMetadata{
		RepositoryURL:  normalizedURL,
		ID:             xqsp.ID,
		Name:           xqsp.Name,
		Version:        xqsp.Version,
		Description:    xqsp.Description,
		Author:         xqsp.Author,
		Homepage:       xqsp.Homepage,
		License:        xqsp.License,
		MinCoreVersion: xqsp.MinCoreVersion,
		Platforms:      platforms,
		Tags:           xqsp.Tags,
		README:         readmeContent,
		LatestRelease:  release.TagName,
		Prerelease:     release.Prerelease,
		PublishedAt:    domainplugin.ParseReleasePublishedAt(release.PublishedAt),
		DownloadCount:  domainplugin.TotalReleaseDownloadCount(release.Assets),
		Manifest:       xqsp.Manifest,
	}

	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	s.setCachedMetadata(ctx, cacheKey, metadata)
	return metadata, nil
}

func (s *GitHubPluginService) fetchManifestForRelease(ctx context.Context, owner, repo, releaseTag string) ([]byte, error) {
	manifestContent, err := s.apiClient.GetFileContent(ctx, owner, repo, domainplugin.XQSPManifestFile, releaseTag)
	if err == nil {
		return manifestContent, nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return nil, err
	}
	manifestContent, err = s.apiClient.GetFileContent(ctx, owner, repo, domainplugin.XQSPManifestFile, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, domainplugin.ErrPluginManifestNotFound
		}
		return nil, err
	}
	return manifestContent, nil
}

func (s *GitHubPluginService) fetchReadmeForRelease(ctx context.Context, owner, repo, releaseTag string) string {
	readmeContent, _ := s.apiClient.GetFileContent(ctx, owner, repo, "README.md", releaseTag)
	if len(readmeContent) == 0 {
		readmeContent, _ = s.apiClient.GetFileContent(ctx, owner, repo, "README.md", "")
	}
	return string(readmeContent)
}
