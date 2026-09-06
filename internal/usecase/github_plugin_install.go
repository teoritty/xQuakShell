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

	return s.commitInstall(ctx, normalizedURL, stageDir, policy, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess)
}

func (s *GitHubPluginService) ensureRepositoryRegistered(ctx context.Context, normalizedURL string) error {
	if _, err := s.storage.Get(ctx, normalizedURL); err != nil {
		return fmt.Errorf("repository not registered: %w", err)
	}
	return nil
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
	grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess bool,
) error {
	installed, err := s.pluginManager.Install(stageDir, policy, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess)
	if err != nil {
		return err
	}

	if err := s.recordInstallConsent(ctx, &installed.Manifest, domainplugin.ConsentFlags{
		SecretAccess:     grantSecretAccess,
		AuthProvider:     grantAuthProviderAccess,
		TunnelProvider:   grantTunnelProviderAccess,
		MultiSession:     grantMultiSessionAccess,
		ArbitraryNetwork: grantArbitraryNetworkAccess,
		ExecChannel:      grantExecAccess,
	}); err != nil {
		return err
	}

	_ = s.storage.UpdateFetchedAt(ctx, normalizedURL, time.Now())
	_ = s.InvalidateMetadataCache(ctx, normalizedURL, "")

	if err := s.pluginManager.EnsureRunning(ctx, installed.Manifest.ID); err != nil {
		slog.Warn("plugin installed but failed to auto-start", "plugin", installed.Manifest.ID, "error", err)
	}

	return nil
}

// recordInstallConsent stores what the user agreed to, as one grant rather than one write per
// elevated capability (ADR-022).
//
// The install preview flags are deliberately not consulted: GrantedPermissions can only keep what
// the manifest actually asked for, so a box ticked for something it never requested grants nothing
// anyway, and repeating the preview conditions here would be a second place for them to drift.
func (s *GitHubPluginService) recordInstallConsent(
	ctx context.Context,
	manifest *domainplugin.Manifest,
	consent domainplugin.ConsentFlags,
) error {
	if s == nil || s.pluginManager == nil || s.pluginManager.pluginSettings == nil || manifest == nil {
		return nil
	}
	granted := domainplugin.GrantedPermissions(manifest, consent)
	return s.pluginManager.pluginSettings.RecordConsent(ctx, manifest.ID, granted)
}
