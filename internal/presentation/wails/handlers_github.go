package wails

import (
	"context"
	"fmt"
)

// ListGitHubRepositories returns all registered GitHub repositories.
func (a *AppAPI) ListGitHubRepositories() ([]GitHubRepositoryDTO, error) {
	if a.githubRepoService == nil {
		return nil, fmt.Errorf("GitHub repository service not available")
	}

	ctx := context.Background()
	repos, err := a.githubRepoService.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}

	dtos := make([]GitHubRepositoryDTO, len(repos))
	for i := range repos {
		dtos[i] = githubRepoToDTO(repos[i])
	}
	return dtos, nil
}

// AddGitHubRepository registers a new GitHub repository.
func (a *AppAPI) AddGitHubRepository(req AddGitHubRepositoryRequest) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}
	ctx := context.Background()
	if _, err := a.githubPluginService.FetchPluginMetadata(ctx, req.URL, true); err != nil {
		return err
	}
	return a.githubRepoService.AddRepository(ctx, req.URL, req.Trusted)
}

// RemoveGitHubRepository unregisters a GitHub repository.
func (a *AppAPI) RemoveGitHubRepository(repoURL string) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	ctx := context.Background()
	return a.githubRepoService.RemoveRepository(ctx, repoURL)
}

// SetGitHubRepositoryTrust marks a repository as trusted or untrusted.
func (a *AppAPI) SetGitHubRepositoryTrust(req SetGitHubRepositoryTrustRequest) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	ctx := context.Background()
	return a.githubRepoService.SetRepositoryTrust(ctx, req.URL, req.Trusted)
}

// FetchGitHubPluginsRequest controls plugin discovery from a repository.
type FetchGitHubPluginsRequest struct {
	URL          string `json:"url"`
	ForceRefresh bool   `json:"forceRefresh"`
}

// FetchGitHubPlugins retrieves available plugins from a repository.
func (a *AppAPI) FetchGitHubPlugins(req FetchGitHubPluginsRequest) (*GitHubPluginListDTO, error) {
	if a.githubPluginService == nil {
		return nil, fmt.Errorf("GitHub plugin service not available")
	}

	ctx := context.Background()
	metadata, err := a.githubPluginService.FetchPluginMetadata(ctx, req.URL, req.ForceRefresh)
	if err != nil {
		return nil, err
	}

	state := installedPluginState{}
	if a.plugins != nil {
		for _, info := range a.plugins.List() {
			if info.ID == metadata.ID {
				state = installedPluginState{
					installed:           true,
					installedVersion:    info.Version,
					installedReleaseTag: info.InstalledReleaseTag,
				}
				break
			}
		}
	}

	dto := &GitHubPluginListDTO{
		RepositoryURL: metadata.RepositoryURL,
		Plugins:       []GitHubPluginMetadataDTO{metadataToDTO(metadata, state)},
	}
	return dto, nil
}

// PreviewGitHubPluginInstall returns install preview and warnings for a GitHub plugin.
func (a *AppAPI) PreviewGitHubPluginInstall(repoURL, releaseTag string) (GitHubPluginPreviewResponseDTO, error) {
	if a.githubPluginService == nil {
		return GitHubPluginPreviewResponseDTO{}, fmt.Errorf("GitHub plugin service not available")
	}
	ctx := context.Background()
	preview, err := a.githubPluginService.PreviewInstall(ctx, repoURL, releaseTag)
	if err != nil {
		return GitHubPluginPreviewResponseDTO{}, err
	}
	return githubPreviewToDTO(preview), nil
}

// InstallGitHubPlugin installs a plugin from GitHub.
func (a *AppAPI) InstallGitHubPlugin(repoURL, releaseTag string, grantSecretAccess bool, grantMultiSessionAccess bool, grantArbitraryNetworkAccess bool) error {
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}

	ctx := context.Background()
	if err := a.githubPluginService.InstallPluginFromGitHub(ctx, repoURL, releaseTag, grantSecretAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess); err != nil {
		return err
	}
	a.EmitPluginContributionsChanged()
	return nil
}

// UninstallGitHubPlugin completely removes a plugin.
func (a *AppAPI) UninstallGitHubPlugin(pluginID string, removeData bool) error {
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}

	ctx := context.Background()
	if err := a.githubPluginService.UninstallPlugin(ctx, pluginID, removeData); err != nil {
		return err
	}
	a.EmitPluginContributionsChanged()
	return nil
}
