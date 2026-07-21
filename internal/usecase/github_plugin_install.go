package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// InstallPluginFromGitHub downloads and installs a plugin from GitHub.
func (s *GitHubPluginService) InstallPluginFromGitHub(
	ctx context.Context,
	repoURL string,
	releaseTag string,
	grantSecretAccess bool,
	grantAuthProviderAccess bool,
	grantTunnelProviderAccess bool,
	grantMultiSessionAccess bool,
	grantArbitraryNetworkAccess bool,
	grantExecAccess bool,
) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}

	if err := s.ensureRepositoryRegistered(ctx, normalizedURL); err != nil {
		return err
	}

	releaseTag = strings.TrimSpace(releaseTag)
	if err := s.validateReleaseTag(ctx, normalizedURL, releaseTag); err != nil {
		return err
	}

	metadata, err := s.FetchPluginMetadataForRelease(ctx, normalizedURL, releaseTag)
	if err != nil {
		return err
	}

	stageDir, cleanup, err := s.downloadAndStage(ctx, normalizedURL, releaseTag, metadata)
	if err != nil {
		return err
	}
	defer cleanup()

	policy, err := InstallTrustPolicy(s.pluginManager.settingsReader)
	if err != nil {
		return err
	}

	preview, err := s.pluginManager.PreviewInstall(stageDir, policy)
	if err != nil {
		return err
	}
	if err := enforceInstallConsents(preview, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess); err != nil {
		return err
	}

	return s.commitInstall(ctx, normalizedURL, stageDir, policy, preview, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess)
}

func (s *GitHubPluginService) ensureRepositoryRegistered(ctx context.Context, normalizedURL string) error {
	if _, err := s.storage.Get(ctx, normalizedURL); err != nil {
		return fmt.Errorf("repository not registered: %w", err)
	}
	return nil
}

func (s *GitHubPluginService) downloadAndStage(
	ctx context.Context,
	normalizedURL, releaseTag string,
	metadata *domainplugin.GitHubPluginMetadata,
) (stageDir string, cleanup func(), err error) {
	platformInfo := metadata.GetPlatformForCurrent()
	if platformInfo == nil {
		return "", func() {}, domainplugin.ErrPlatformNotSupported
	}

	owner, repoName, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return "", func() {}, err
	}

	binaryPath, binaryCleanup, err := s.downloadBinary(ctx, owner, repoName, metadata.LatestRelease, platformInfo.AssetName, platformInfo.Checksum)
	if err != nil {
		return "", func() {}, err
	}
	defer binaryCleanup()

	stageDir, stageCleanup, err := s.stager(binaryPath, metadata.Manifest)
	if err != nil {
		return "", func() {}, err
	}

	installTag := releaseTag
	if installTag == "" {
		installTag = metadata.LatestRelease
	}
	if s.installMetaWriter != nil {
		if err := s.installMetaWriter.Write(stageDir, domainplugin.PluginInstallMeta{
			Source:        domainplugin.InstallMetaSourceGitHub,
			RepositoryURL: normalizedURL,
			ReleaseTag:    installTag,
		}); err != nil {
			stageCleanup()
			return "", func() {}, err
		}
	}

	return stageDir, stageCleanup, nil
}

func enforceInstallConsents(
	preview InstallPreview,
	grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess bool,
) error {
	if preview.RequiresSecretAccess && !grantSecretAccess {
		return fmt.Errorf("secret access consent required for this plugin")
	}
	if preview.RequiresAuthProviderAccess && !grantAuthProviderAccess {
		return fmt.Errorf("auth provider consent required for this plugin")
	}
	if preview.RequiresTunnelProviderAccess && !grantTunnelProviderAccess {
		return fmt.Errorf("tunnel provider consent required for this plugin")
	}
	if preview.MultiSessionWarning && !grantMultiSessionAccess {
		return fmt.Errorf("multi-session consent required for this plugin")
	}
	if preview.ArbitraryNetworkWarning && !grantArbitraryNetworkAccess {
		return fmt.Errorf("arbitrary network access consent required for this plugin")
	}
	if preview.ExecAccessWarning && !grantExecAccess {
		return fmt.Errorf("exec channel consent required for this plugin")
	}
	return nil
}

func (s *GitHubPluginService) commitInstall(
	ctx context.Context,
	normalizedURL string,
	stageDir string,
	policy domainplugin.InstallTrustPolicy,
	preview InstallPreview,
	grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess bool,
) error {
	installed, err := s.pluginManager.Install(stageDir, policy, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess)
	if err != nil {
		return err
	}

	if preview.RequiresSecretAccess && grantSecretAccess && s.pluginManager.pluginSettings != nil {
		if err := s.pluginManager.pluginSettings.GrantSecretAccess(ctx, installed.Manifest.ID); err != nil {
			return err
		}
	}
	if preview.RequiresAuthProviderAccess && grantAuthProviderAccess && s.pluginManager.pluginSettings != nil {
		if err := s.pluginManager.pluginSettings.GrantAuthProviderAccess(ctx, installed.Manifest.ID); err != nil {
			return err
		}
	}
	if preview.RequiresTunnelProviderAccess && grantTunnelProviderAccess && s.pluginManager.pluginSettings != nil {
		if err := s.pluginManager.pluginSettings.GrantTunnelProviderAccess(ctx, installed.Manifest.ID); err != nil {
			return err
		}
	}
	if preview.ArbitraryNetworkWarning && grantArbitraryNetworkAccess && s.pluginManager.pluginSettings != nil {
		if err := s.pluginManager.pluginSettings.GrantArbitraryNetworkAccess(ctx, installed.Manifest.ID); err != nil {
			return err
		}
	}

	_ = s.storage.UpdateFetchedAt(ctx, normalizedURL, time.Now())
	_ = s.InvalidateMetadataCache(ctx, normalizedURL, "")

	if err := s.pluginManager.EnsureRunning(ctx, installed.Manifest.ID); err != nil {
		slog.Warn("plugin installed but failed to auto-start", "plugin", installed.Manifest.ID, "error", err)
	}

	return nil
}
