package wails

import (
	"fmt"
)

func (a *AppAPI) ListGitHubRepositories() ([]GitHubRepositoryDTO, error) {
	if a.githubRepoService == nil {
		return nil, fmt.Errorf("GitHub repository service not available")
	}

	ctx := a.reqCtx()
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

func (a *AppAPI) AddGitHubRepository(req AddGitHubRepositoryRequest) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}
	ctx := a.reqCtx()
	if _, err := a.githubPluginService.FetchPluginMetadata(ctx, req.URL, true); err != nil {
		return err
	}
	return a.githubRepoService.AddRepository(ctx, req.URL, req.Trusted)
}

func (a *AppAPI) RemoveGitHubRepository(repoURL string) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	ctx := a.reqCtx()
	return a.githubRepoService.RemoveRepository(ctx, repoURL)
}

func (a *AppAPI) SetGitHubRepositoryTrust(req SetGitHubRepositoryTrustRequest) error {
	if a.githubRepoService == nil {
		return fmt.Errorf("GitHub repository service not available")
	}
	ctx := a.reqCtx()
	return a.githubRepoService.SetRepositoryTrust(ctx, req.URL, req.Trusted)
}

type FetchGitHubPluginsRequest struct {
	URL          string `json:"url"`
	ForceRefresh bool   `json:"forceRefresh"`
}

func (a *AppAPI) FetchGitHubPlugins(req FetchGitHubPluginsRequest) (*GitHubPluginListDTO, error) {
	if a.githubPluginService == nil {
		return nil, fmt.Errorf("GitHub plugin service not available")
	}

	ctx := a.reqCtx()
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

func (a *AppAPI) PreviewGitHubPluginInstall(repoURL, releaseTag string) (GitHubPluginPreviewResponseDTO, error) {
	if a.githubPluginService == nil {
		return GitHubPluginPreviewResponseDTO{}, fmt.Errorf("GitHub plugin service not available")
	}
	ctx := a.reqCtx()
	preview, err := a.githubPluginService.PreviewInstall(ctx, repoURL, releaseTag)
	if err != nil {
		return GitHubPluginPreviewResponseDTO{}, err
	}
	return githubPreviewToDTO(preview), nil
}

func (a *AppAPI) InstallGitHubPlugin(repoURL, releaseTag string, grantSecretAccess bool, grantAuthProviderAccess bool, grantTunnelProviderAccess bool, grantMultiSessionAccess bool, grantArbitraryNetworkAccess bool, grantExecAccess bool) error {
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}

	ctx := a.reqCtx()
	if err := a.githubPluginService.InstallPluginFromGitHub(ctx, repoURL, releaseTag, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess); err != nil {
		return err
	}
	a.EmitPluginContributionsChanged()
	return nil
}

func (a *AppAPI) UninstallGitHubPlugin(pluginID string, removeData bool) error {
	if a.githubPluginService == nil {
		return fmt.Errorf("GitHub plugin service not available")
	}

	ctx := a.reqCtx()
	if err := a.githubPluginService.UninstallPlugin(ctx, pluginID, removeData); err != nil {
		return err
	}
	a.EmitPluginContributionsChanged()
	return nil
}
