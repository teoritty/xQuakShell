package usecase

import (
	"fmt"
	"strings"

	domainplugin "ssh-client/internal/domain/plugin"
)

// GitHubPluginPreviewDTO contains information shown before installation.
type GitHubPluginPreviewDTO struct {
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
	PublishedDate        string   `json:"publishedDate"`
	README               string   `json:"readme"`
	RequiresSecretAccess bool     `json:"requiresSecretAccess"`
	UnsignedPlugin       bool     `json:"unsignedPlugin"`
	UntrustedSource      bool     `json:"untrustedSource"`
	Warnings             []string `json:"warnings"`
}

// BuildPreviewDTO creates a preview DTO from metadata.
func BuildPreviewDTO(metadata *domainplugin.GitHubPluginMetadata, repoTrusted, unsignedPlugin bool) GitHubPluginPreviewDTO {
	currentOS := domainplugin.CurrentPlatformOS()
	currentArch := domainplugin.CurrentPlatformArch()
	currentPlatform := currentOS + "/" + currentArch

	supportedPlatforms := make([]string, 0, len(metadata.Platforms))
	for _, p := range metadata.Platforms {
		supportedPlatforms = append(supportedPlatforms, p.OS+"/"+p.Arch)
	}

	var warnings []string
	untrustedSource := !repoTrusted
	if untrustedSource {
		warnings = append(warnings, "This plugin is from an untrusted source")
	}
	if unsignedPlugin {
		warnings = append(warnings, "This plugin is not signed")
	}
	if metadata.RequiresSecretAccess() {
		warnings = append(warnings, "This plugin can access secrets")
	}

	return GitHubPluginPreviewDTO{
		RepositoryURL:        metadata.RepositoryURL,
		RepositoryTrusted:    repoTrusted,
		ID:                   metadata.ID,
		Name:                 metadata.Name,
		Version:              metadata.Version,
		Description:          metadata.Description,
		Author:               metadata.Author,
		License:              metadata.License,
		MinCoreVersion:       metadata.MinCoreVersion,
		CurrentPlatform:      currentPlatform,
		PlatformSupported:    metadata.SupportsCurrentPlatform(),
		SupportedPlatforms:   supportedPlatforms,
		LatestRelease:        metadata.LatestRelease,
		PublishedDate:        metadata.PublishedAt,
		README:               metadata.README,
		RequiresSecretAccess: metadata.RequiresSecretAccess(),
		UnsignedPlugin:       unsignedPlugin,
		UntrustedSource:      untrustedSource,
		Warnings:             warnings,
	}
}

// InstallTrustPolicy builds trust policy from vault plugin settings.
func InstallTrustPolicy(reader PluginSettingsReader) (domainplugin.InstallTrustPolicy, error) {
	policy := domainplugin.InstallTrustPolicy{}
	if reader == nil {
		return policy, nil
	}
	settings, err := reader.PluginSettings()
	if err != nil {
		return policy, err
	}
	policy.RequireSigned = settings.RequireSignedPlugins
	keys, err := domainplugin.ParseTrustedPublisherKeys(settings.TrustedPublisherKeys)
	if err != nil {
		return policy, fmt.Errorf("trusted publisher keys: %w", err)
	}
	policy.TrustedKeys = keys
	return policy, nil
}

func parseGitHubAssetName(filename string) (osName, arch string) {
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
	if !domainplugin.IsValidPlatformOS(osName) || !domainplugin.IsValidPlatformArch(arch) {
		return "", ""
	}
	return osName, arch
}
