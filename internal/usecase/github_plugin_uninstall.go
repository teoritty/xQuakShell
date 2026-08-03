package usecase

import (
	"context"
)

// UninstallPlugin completely removes a user-installed plugin and optionally its data.
func (s *GitHubPluginService) UninstallPlugin(ctx context.Context, pluginID string, removeData bool) error {
	var repoURL string
	if s.pluginManager != nil {
		if plugin, err := s.pluginManager.Registry().Get(pluginID); err == nil && plugin.InstallMeta != nil {
			repoURL = plugin.InstallMeta.RepositoryURL
		}
	}
	if err := s.pluginManager.UninstallPlugin(ctx, pluginID, removeData); err != nil {
		return err
	}
	if repoURL != "" {
		_ = s.InvalidateMetadataCache(ctx, repoURL, "")
	}
	return nil
}
