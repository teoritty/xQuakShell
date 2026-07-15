package usecase

import (
	"context"
	"fmt"

	domainplugin "ssh-client/internal/domain/plugin"
)

// GitHubPluginPreviewDTO contains information shown before installation.
type GitHubPluginPreviewDTO struct {
	RepositoryURL                string   `json:"repositoryUrl"`
	RepositoryTrusted            bool     `json:"repositoryTrusted"`
	ID                           string   `json:"id"`
	Name                         string   `json:"name"`
	Version                      string   `json:"version"`
	Description                  string   `json:"description"`
	Author                       string   `json:"author"`
	License                      string   `json:"license"`
	MinCoreVersion               string   `json:"minCoreVersion"`
	CurrentPlatform              string   `json:"currentPlatform"`
	PlatformSupported            bool     `json:"platformSupported"`
	SupportedPlatforms           []string `json:"supportedPlatforms"`
	LatestRelease                string   `json:"latestRelease"`
	ReleaseTag                   string   `json:"releaseTag"`
	Prerelease                   bool     `json:"prerelease"`
	PublishedDate                string   `json:"publishedDate"`
	README                       string   `json:"readme"`
	RequiresSecretAccess         bool     `json:"requiresSecretAccess"`
	RequiresAuthProviderAccess   bool     `json:"requiresAuthProviderAccess"`
	RequiresTunnelProviderAccess bool     `json:"requiresTunnelProviderAccess"`
	MultiSessionWarning          bool     `json:"multiSessionWarning"`
	ArbitraryNetworkWarning      bool     `json:"arbitraryNetworkWarning"`
	UnsignedPlugin               bool     `json:"unsignedPlugin"`
	UntrustedSource              bool     `json:"untrustedSource"`
	// Compatible reports whether this host can satisfy the plugin's declared API/capability
	// requirements (ADR-012). When false, CompatibilityIssues lists exactly what is missing.
	Compatible          bool     `json:"compatible"`
	CompatibilityIssues []string `json:"compatibilityIssues"`
	Warnings            []string `json:"warnings"`
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
	if metadata.Manifest.RequiresAuthProviderAccess() {
		warnings = append(warnings, "This plugin can participate in SSH authentication")
	}
	if metadata.Manifest.RequiresTunnelProviderAccess() {
		warnings = append(warnings, "This plugin can route dynamic port-forward connections")
	}
	arbitraryNetworkWarning := metadata.Manifest.RequiresArbitraryNetworkAccess()
	if arbitraryNetworkWarning {
		warnings = append(warnings, "This plugin can connect to any host on the network")
		if metadata.Manifest.RequiresPrivateNetworkAccess() {
			warnings = append(warnings, "Including private and local network addresses")
		}
	}

	// Resolve the plugin's API/capability requirements against this host so the install UI can
	// warn (and the user can decide) before download (ADR-012). Any migration warnings are
	// surfaced too; a hard incompatibility blocks with the exact missing items.
	compatible := true
	var compatibilityIssues []string
	manifest := metadata.Manifest
	if eff, warns, err := domainplugin.EffectiveRequirements(&manifest); err != nil {
		compatible = false
		compatibilityIssues = append(compatibilityIssues, err.Error())
	} else {
		warnings = append(warnings, warns...)
		if report := eff.CheckAgainstHost(domainplugin.HostRegistry()); report != nil {
			compatible = false
			for _, it := range report.Items {
				compatibilityIssues = append(compatibilityIssues, it.Detail)
			}
		}
	}
	if !compatible {
		warnings = append(warnings, "This plugin is not compatible with your version of xQuakShell")
	}

	return GitHubPluginPreviewDTO{
		RepositoryURL:                metadata.RepositoryURL,
		RepositoryTrusted:            repoTrusted,
		ID:                           metadata.ID,
		Name:                         metadata.Name,
		Version:                      metadata.Version,
		Description:                  metadata.Description,
		Author:                       metadata.Author,
		License:                      metadata.License,
		MinCoreVersion:               metadata.MinCoreVersion,
		CurrentPlatform:              currentPlatform,
		PlatformSupported:            metadata.SupportsCurrentPlatform(),
		SupportedPlatforms:           supportedPlatforms,
		LatestRelease:                metadata.LatestRelease,
		ReleaseTag:                   metadata.LatestRelease,
		Prerelease:                   metadata.Prerelease,
		PublishedDate:                metadata.PublishedAt,
		README:                       metadata.README,
		RequiresSecretAccess:         metadata.RequiresSecretAccess(),
		RequiresAuthProviderAccess:   metadata.Manifest.RequiresAuthProviderAccess(),
		RequiresTunnelProviderAccess: metadata.Manifest.RequiresTunnelProviderAccess(),
		MultiSessionWarning:          metadata.Manifest.RequiresMultiSessionWarning(),
		ArbitraryNetworkWarning:      arbitraryNetworkWarning,
		UnsignedPlugin:               unsignedPlugin,
		UntrustedSource:              untrustedSource,
		Compatible:                   compatible,
		CompatibilityIssues:          compatibilityIssues,
		Warnings:                     warnings,
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

// PreviewInstall builds install preview information for a GitHub plugin.
func (s *GitHubPluginService) PreviewInstall(ctx context.Context, repoURL, releaseTag string) (GitHubPluginPreviewDTO, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return GitHubPluginPreviewDTO{}, err
	}

	metadata, err := s.FetchPluginMetadataForRelease(ctx, normalizedURL, releaseTag)
	if err != nil {
		return GitHubPluginPreviewDTO{}, err
	}

	repoTrusted := false
	if repo, err := s.storage.Get(ctx, normalizedURL); err == nil && repo != nil {
		repoTrusted = repo.Trusted
	}

	policy, err := InstallTrustPolicy(s.pluginManager.settingsReader)
	if err != nil {
		return GitHubPluginPreviewDTO{}, err
	}

	unsignedPlugin := metadata.Manifest.Signature == ""
	if metadata.Manifest.Signature != "" {
		trust, err := domainplugin.EvaluateInstallTrust(metadata.Manifest, "", policy)
		if err == nil {
			unsignedPlugin = trust.UnsignedWarning || trust.UntrustedSignatureWarning
		}
	}

	return BuildPreviewDTO(metadata, repoTrusted, unsignedPlugin), nil
}
