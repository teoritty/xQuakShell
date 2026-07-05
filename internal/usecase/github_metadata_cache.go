package usecase

import (
	"context"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// metadataCacheKey builds a cache key for repository metadata.
func metadataCacheKey(normalizedURL, releaseTag string) string {
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag == "" {
		return "metadata:" + normalizedURL
	}
	return "metadata:" + normalizedURL + ":" + releaseTag
}

func (s *GitHubPluginService) getCachedMetadata(ctx context.Context, cacheKey string) (*domainplugin.GitHubPluginMetadata, bool) {
	if s.cache == nil {
		return nil, false
	}
	cached, found, err := s.cache.Get(ctx, cacheKey)
	if err != nil || !found {
		return nil, false
	}
	meta, ok := cached.(*domainplugin.GitHubPluginMetadata)
	if !ok {
		return nil, false
	}
	return meta, true
}

func (s *GitHubPluginService) setCachedMetadata(ctx context.Context, cacheKey string, metadata *domainplugin.GitHubPluginMetadata) {
	if s.cache == nil || metadata == nil {
		return
	}
	_ = s.cache.Set(ctx, cacheKey, metadata)
}

// InvalidateMetadataCache removes cached metadata for a repository and optional release tag.
func (s *GitHubPluginService) InvalidateMetadataCache(ctx context.Context, repoURL, releaseTag string) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag != "" {
		return s.cache.Delete(ctx, metadataCacheKey(normalizedURL, releaseTag))
	}
	if err := s.cache.Delete(ctx, metadataCacheKey(normalizedURL, "")); err != nil {
		return err
	}
	return s.cache.DeletePrefix(ctx, "metadata:"+normalizedURL+":")
}
