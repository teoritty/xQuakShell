package wails

import (
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/usecase"
)

// GitHubRepositoryDTO represents a registered GitHub repository.
type GitHubRepositoryDTO struct {
	URL           string `json:"url"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	DisplayName   string `json:"displayName"`
	AddedAt       string `json:"addedAt"`
	LastFetchedAt string `json:"lastFetchedAt,omitempty"`
	Trusted       bool   `json:"trusted"`
}

func githubRepoToDTO(repo domainplugin.GitHubRepository) GitHubRepositoryDTO {
	dto := GitHubRepositoryDTO{
		URL:         repo.URL,
		Owner:       repo.Owner,
		Repo:        repo.Repo,
		DisplayName: repo.DisplayName,
		AddedAt:     repo.AddedAt.Format(time.RFC3339),
		Trusted:     repo.Trusted,
	}
	if repo.LastFetchedAt != nil {
		dto.LastFetchedAt = repo.LastFetchedAt.Format(time.RFC3339)
	}
	return dto
}

// AddGitHubRepositoryRequest is the request to add a repository.
type AddGitHubRepositoryRequest struct {
	URL     string `json:"url"`
	Trusted bool   `json:"trusted"`
}

// SetGitHubRepositoryTrustRequest toggles repository trust.
type SetGitHubRepositoryTrustRequest struct {
	URL     string `json:"url"`
	Trusted bool   `json:"trusted"`
}

// GitHubPluginListDTO contains plugins available from a repository.
type GitHubPluginListDTO struct {
	RepositoryURL string                    `json:"repositoryUrl"`
	Plugins       []GitHubPluginMetadataDTO `json:"plugins"`
}

// GitHubReleaseSummaryDTO describes a published GitHub release.
type GitHubReleaseSummaryDTO struct {
	Tag               string            `json:"tag"`
	Name              string            `json:"name"`
	PublishedAt       string            `json:"publishedAt"`
	Prerelease        bool              `json:"prerelease"`
	PlatformSupported bool              `json:"platformSupported"`
	Platforms         []PlatformInfoDTO `json:"platforms"`
}

// GitHubPluginMetadataDTO represents plugin metadata for UI.
type GitHubPluginMetadataDTO struct {
	RepositoryURL     string                    `json:"repositoryUrl"`
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Version           string                    `json:"version"`
	Description       string                    `json:"description"`
	Author            string                    `json:"author"`
	License           string                    `json:"license"`
	Platforms         []PlatformInfoDTO         `json:"platforms"`
	AvailableReleases []GitHubReleaseSummaryDTO `json:"availableReleases"`
	LatestRelease     string                    `json:"latestRelease"`
	Prerelease        bool                      `json:"prerelease"`
	PublishedAt       string                    `json:"publishedAt"`
	README            string                    `json:"readme"`
	MinCoreVersion    string                    `json:"minCoreVersion"`
	PlatformSupported bool                      `json:"platformSupported"`
	Installed         bool                      `json:"installed"`
}

// PlatformInfoDTO represents platform support info.
type PlatformInfoDTO struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	AssetName string `json:"assetName"`
}

// GitHubPluginPreviewResponseDTO is the install preview for GitHub plugins.
type GitHubPluginPreviewResponseDTO struct {
	RepositoryURL        string   `json:"repositoryUrl"`
	RepositoryTrusted    bool     `json:"repositoryTrusted"`
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Version              string   `json:"version"`
	Description          string   `json:"description"`
	Author               string   `json:"author"`
	License              string   `json:"license"`
	MinCoreVersion       string   `json:"minCoreVersion"`
	CurrentPlatform      string   `json:"currentPlatform"`
	PlatformSupported    bool     `json:"platformSupported"`
	SupportedPlatforms   []string `json:"supportedPlatforms"`
	LatestRelease        string   `json:"latestRelease"`
	ReleaseTag           string   `json:"releaseTag"`
	Prerelease           bool     `json:"prerelease"`
	PublishedDate        string   `json:"publishedDate"`
	README               string   `json:"readme"`
	RequiresSecretAccess    bool     `json:"requiresSecretAccess"`
	MultiSessionWarning     bool     `json:"multiSessionWarning"`
	ArbitraryNetworkWarning bool     `json:"arbitraryNetworkWarning"`
	UnsignedPlugin          bool     `json:"unsignedPlugin"`
	UntrustedSource      bool     `json:"untrustedSource"`
	Warnings             []string `json:"warnings"`
}

func githubPreviewToDTO(p usecase.GitHubPluginPreviewDTO) GitHubPluginPreviewResponseDTO {
	warnings := p.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	supported := p.SupportedPlatforms
	if supported == nil {
		supported = []string{}
	}
	return GitHubPluginPreviewResponseDTO{
		RepositoryURL:        p.RepositoryURL,
		RepositoryTrusted:    p.RepositoryTrusted,
		ID:                   p.ID,
		Name:                 p.Name,
		Version:              p.Version,
		Description:          p.Description,
		Author:               p.Author,
		License:              p.License,
		MinCoreVersion:       p.MinCoreVersion,
		CurrentPlatform:      p.CurrentPlatform,
		PlatformSupported:    p.PlatformSupported,
		SupportedPlatforms:   supported,
		LatestRelease:        p.LatestRelease,
		ReleaseTag:           p.ReleaseTag,
		Prerelease:           p.Prerelease,
		PublishedDate:        p.PublishedDate,
		README:               p.README,
		RequiresSecretAccess:    p.RequiresSecretAccess,
		MultiSessionWarning:     p.MultiSessionWarning,
		ArbitraryNetworkWarning: p.ArbitraryNetworkWarning,
		UnsignedPlugin:          p.UnsignedPlugin,
		UntrustedSource:      p.UntrustedSource,
		Warnings:             warnings,
	}
}

func metadataToDTO(metadata *domainplugin.GitHubPluginMetadata, installed bool) GitHubPluginMetadataDTO {
	dto := GitHubPluginMetadataDTO{
		RepositoryURL:     metadata.RepositoryURL,
		ID:                metadata.ID,
		Name:              metadata.Name,
		Version:           metadata.Version,
		Description:       metadata.Description,
		Author:            metadata.Author,
		License:           metadata.License,
		LatestRelease:     metadata.LatestRelease,
		Prerelease:        metadata.Prerelease,
		PublishedAt:       metadata.PublishedAt,
		README:            metadata.README,
		MinCoreVersion:    metadata.MinCoreVersion,
		PlatformSupported: metadata.SupportsCurrentPlatform(),
		Installed:         installed,
	}
	for _, p := range metadata.Platforms {
		dto.Platforms = append(dto.Platforms, PlatformInfoDTO{
			OS:        p.OS,
			Arch:      p.Arch,
			AssetName: p.AssetName,
		})
	}
	for _, release := range metadata.AvailableReleases {
		summary := GitHubReleaseSummaryDTO{
			Tag:               release.Tag,
			Name:              release.Name,
			PublishedAt:       release.PublishedAt,
			Prerelease:        release.Prerelease,
			PlatformSupported: release.PlatformSupported,
		}
		for _, p := range release.Platforms {
			summary.Platforms = append(summary.Platforms, PlatformInfoDTO{
				OS:        p.OS,
				Arch:      p.Arch,
				AssetName: p.AssetName,
			})
		}
		dto.AvailableReleases = append(dto.AvailableReleases, summary)
	}
	return dto
}
