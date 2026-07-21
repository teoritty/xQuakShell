package usecase

import (
	domainplugin "xquakshell/internal/domain/plugin"
)

// GitHubPluginService orchestrates GitHub plugin discovery, preview, install, and uninstall.
type GitHubPluginService struct {
	apiClient         GitHubAPIClient
	downloader        PluginBinaryDownloader
	stager            GitHubPluginStager
	installMetaWriter GitHubInstallMetaWriter
	cache             domainplugin.GitHubCache
	pluginManager     *PluginManager
	storage           domainplugin.GitHubRepositoryStorage
}

// NewGitHubPluginService creates a new GitHub plugin service.
func NewGitHubPluginService(
	apiClient GitHubAPIClient,
	downloader PluginBinaryDownloader,
	stager GitHubPluginStager,
	installMetaWriter GitHubInstallMetaWriter,
	cache domainplugin.GitHubCache,
	pluginManager *PluginManager,
	storage domainplugin.GitHubRepositoryStorage,
) *GitHubPluginService {
	return &GitHubPluginService{
		apiClient:         apiClient,
		downloader:        downloader,
		stager:            stager,
		installMetaWriter: installMetaWriter,
		cache:             cache,
		pluginManager:     pluginManager,
		storage:           storage,
	}
}
