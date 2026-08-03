package usecase

import (
	"context"
	"fmt"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
)

func (s *GitHubPluginService) validateReleaseTag(ctx context.Context, repoURL, releaseTag string) error {
	releaseTag = strings.TrimSpace(releaseTag)
	if releaseTag == "" {
		return nil
	}
	metadata, err := s.FetchPluginMetadata(ctx, repoURL, false)
	if err != nil {
		return err
	}
	for _, release := range metadata.AvailableReleases {
		if release.Tag == releaseTag {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", domainplugin.ErrInvalidReleaseTag, releaseTag)
}
