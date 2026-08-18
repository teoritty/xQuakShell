package usecase

import (
	"context"
	"fmt"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
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

	repoRef, err := domainplugin.ParseRepoRef(normalizedURL)
	if err != nil {
		return nil, err
	}

	manifestContent, err := s.apiClient.GetFileContent(ctx, repoRef, domainplugin.XQSPManifestFile, "")
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

	readmeContent, _ := s.apiClient.GetFileContent(ctx, repoRef, "README.md", "")

	releases, err := s.apiClient.ListPublishedReleases(ctx, repoRef)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%w in %s", domainplugin.ErrNoReleases, repoRef.ProjectPath())
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

	repoRef, err := domainplugin.ParseRepoRef(normalizedURL)
	if err != nil {
		return nil, err
	}

	release, err := s.apiClient.GetReleaseByTag(ctx, repoRef, releaseTag)
	if err != nil {
		return nil, err
	}

	manifestContent, err := s.fetchManifestForRelease(ctx, repoRef, releaseTag)
	if err != nil {
		return nil, err
	}

	xqsp, err := domainplugin.ParseXQSPManifest(manifestContent)
	if err != nil {
		return nil, err
	}

	readmeContent := s.fetchReadmeForRelease(ctx, repoRef, releaseTag)
	checksums, err := s.loadReleaseChecksums(ctx, repoRef, release)
	if err != nil {
		return nil, err
	}
	platforms := domainplugin.ExtractPlatformsFromAssets(release.Assets, checksums)
	if len(platforms) == 0 {
		return nil, fmt.Errorf("%w: no release assets match supported platform naming", domainplugin.ErrInvalidPluginMetadata)
	}

	metadata := &domainplugin.GitHubPluginMetadata{
		RepositoryURL: normalizedURL,
		ID:            xqsp.ID,
		Name:          xqsp.Name,
		Version:       xqsp.Version,
		Description:   xqsp.Description,
		Author:        xqsp.Author,
		Homepage:      xqsp.Homepage,
		License:       xqsp.License,
		Platforms:     platforms,
		Tags:          xqsp.Tags,
		README:        readmeContent,
		LatestRelease: release.TagName,
		Prerelease:    release.Prerelease,
		PublishedAt:   domainplugin.ParseReleasePublishedAt(release.PublishedAt),
		DownloadCount: domainplugin.TotalReleaseDownloadCount(release.Assets),
		Manifest:      xqsp.Manifest,
	}

	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	s.setCachedMetadata(ctx, cacheKey, metadata)
	return metadata, nil
}

func (s *GitHubPluginService) fetchManifestForRelease(ctx context.Context, repoRef domainplugin.RepoRef, releaseTag string) ([]byte, error) {
	manifestContent, err := s.apiClient.GetFileContent(ctx, repoRef, domainplugin.XQSPManifestFile, releaseTag)
	if err == nil {
		return manifestContent, nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return nil, err
	}
	manifestContent, err = s.apiClient.GetFileContent(ctx, repoRef, domainplugin.XQSPManifestFile, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, domainplugin.ErrPluginManifestNotFound
		}
		return nil, err
	}
	return manifestContent, nil
}

func (s *GitHubPluginService) fetchReadmeForRelease(ctx context.Context, repoRef domainplugin.RepoRef, releaseTag string) string {
	readmeContent, _ := s.apiClient.GetFileContent(ctx, repoRef, "README.md", releaseTag)
	if len(readmeContent) == 0 {
		readmeContent, _ = s.apiClient.GetFileContent(ctx, repoRef, "README.md", "")
	}
	return string(readmeContent)
}
