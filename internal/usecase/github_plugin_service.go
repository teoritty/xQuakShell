package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infragithub "ssh-client/internal/infra/github"
	infraplugin "ssh-client/internal/infra/plugin"
)

// GitHubPluginService handles plugin discovery and installation from GitHub.
type GitHubPluginService struct {
	apiClient     GitHubAPIClient
	downloader    PluginBinaryDownloader
	stager        GitHubPluginStager
	cache         domainplugin.GitHubCache
	pluginManager *PluginManager
	storage       domainplugin.GitHubRepositoryStorage
	dataRoot      string
}

// NewGitHubPluginService creates a new GitHub plugin service.
func NewGitHubPluginService(
	apiClient GitHubAPIClient,
	downloader PluginBinaryDownloader,
	stager GitHubPluginStager,
	cache domainplugin.GitHubCache,
	pluginManager *PluginManager,
	storage domainplugin.GitHubRepositoryStorage,
	dataRoot string,
) *GitHubPluginService {
	if stager == nil {
		stager = infraplugin.StageGitHubPlugin
	}
	return &GitHubPluginService{
		apiClient:     apiClient,
		downloader:    downloader,
		stager:        stager,
		cache:         cache,
		pluginManager: pluginManager,
		storage:       storage,
		dataRoot:      dataRoot,
	}
}

// FetchPluginMetadata retrieves plugin metadata from a GitHub repository.
func (s *GitHubPluginService) FetchPluginMetadata(ctx context.Context, repoURL string) (*domainplugin.GitHubPluginMetadata, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return nil, err
	}

	cacheKey := "metadata:" + normalizedURL
	if cached, found, err := s.cache.Get(ctx, cacheKey); err == nil && found {
		if meta, ok := cached.(*domainplugin.GitHubPluginMetadata); ok {
			return meta, nil
		}
	}

	owner, repo, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return nil, err
	}

	manifestContent, err := s.apiClient.GetFileContent(ctx, owner, repo, domainplugin.XQSPManifestFile)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, domainplugin.ErrPluginManifestNotFound
		}
		return nil, err
	}

	xqsp, err := domainplugin.ParseXQSPManifest(manifestContent)
	if err != nil {
		return nil, err
	}

	readmeContent, _ := s.apiClient.GetFileContent(ctx, owner, repo, "README.md")

	latestRelease, err := s.apiClient.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	checksums := s.loadReleaseChecksums(ctx, owner, repo, latestRelease)
	platforms := extractPlatformsFromRelease(latestRelease.Assets, checksums)

	metadata := &domainplugin.GitHubPluginMetadata{
		RepositoryURL:  normalizedURL,
		ID:             xqsp.ID,
		Name:           xqsp.Name,
		Version:        xqsp.Version,
		Description:    xqsp.Description,
		Author:         xqsp.Author,
		Homepage:       xqsp.Homepage,
		License:        xqsp.License,
		MinCoreVersion: xqsp.MinCoreVersion,
		Platforms:      platforms,
		Tags:           xqsp.Tags,
		README:         string(readmeContent),
		LatestRelease:  latestRelease.TagName,
		PublishedAt:    infragithub.ParseReleasePublishedAt(latestRelease.PublishedAt),
		DownloadCount:  infragithub.TotalDownloadCount(latestRelease.Assets),
		Manifest:       xqsp.Manifest,
	}

	if err := metadata.Validate(); err != nil {
		return nil, err
	}

	_ = s.cache.Set(ctx, cacheKey, metadata)
	return metadata, nil
}

// PreviewInstall builds install preview information for a GitHub plugin.
func (s *GitHubPluginService) PreviewInstall(ctx context.Context, repoURL string) (GitHubPluginPreviewDTO, error) {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return GitHubPluginPreviewDTO{}, err
	}

	metadata, err := s.FetchPluginMetadata(ctx, normalizedURL)
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

// InstallPluginFromGitHub downloads and installs a plugin from GitHub.
func (s *GitHubPluginService) InstallPluginFromGitHub(
	ctx context.Context,
	repoURL string,
	grantSecretAccess bool,
	grantMultiSessionAccess bool,
) error {
	normalizedURL, err := domainplugin.NormalizeURL(repoURL)
	if err != nil {
		return err
	}

	if _, err := s.storage.Get(ctx, normalizedURL); err != nil {
		return fmt.Errorf("repository not registered: %w", err)
	}

	metadata, err := s.FetchPluginMetadata(ctx, normalizedURL)
	if err != nil {
		return err
	}

	platformInfo := metadata.GetPlatformForCurrent()
	if platformInfo == nil {
		return domainplugin.ErrPlatformNotSupported
	}

	owner, repoName, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return err
	}

	binaryPath, cleanup, err := s.downloadBinary(ctx, owner, repoName, metadata.LatestRelease, platformInfo.AssetName, platformInfo.Checksum)
	if err != nil {
		return err
	}
	defer cleanup()

	stageDir, err := s.stager(binaryPath, metadata.Manifest)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	policy, err := InstallTrustPolicy(s.pluginManager.settingsReader)
	if err != nil {
		return err
	}

	preview, err := s.pluginManager.PreviewInstall(stageDir, policy)
	if err != nil {
		return err
	}
	if preview.RequiresSecretAccess && !grantSecretAccess {
		return fmt.Errorf("secret access consent required for this plugin")
	}
	if preview.MultiSessionWarning && !grantMultiSessionAccess {
		return fmt.Errorf("multi-session consent required for this plugin")
	}

	installed, err := s.pluginManager.Install(stageDir, policy, grantMultiSessionAccess)
	if err != nil {
		return err
	}

	if preview.RequiresSecretAccess && grantSecretAccess && s.pluginManager.pluginSettings != nil {
		if err := s.pluginManager.pluginSettings.GrantSecretAccess(ctx, installed.Manifest.ID); err != nil {
			return err
		}
	}

	_ = s.storage.UpdateFetchedAt(ctx, normalizedURL, time.Now())

	if err := s.pluginManager.EnsureRunning(ctx, installed.Manifest.ID); err != nil {
		slog.Warn("plugin installed but failed to auto-start", "plugin", installed.Manifest.ID, "error", err)
	}

	return nil
}

// UninstallPlugin completely removes a user-installed plugin and optionally its data.
func (s *GitHubPluginService) UninstallPlugin(ctx context.Context, pluginID string, removeData bool) error {
	return s.pluginManager.UninstallPlugin(ctx, pluginID, removeData)
}

func (s *GitHubPluginService) downloadBinary(
	ctx context.Context,
	owner, repo, tag, assetName, checksum string,
) (path string, cleanup func(), err error) {
	path, err = s.downloader.DownloadBinary(ctx, owner, repo, tag, assetName, checksum)
	if err != nil {
		return "", func() {}, err
	}
	root := findTempRoot(path)
	return path, func() { _ = os.RemoveAll(root) }, nil
}

func findTempRoot(path string) string {
	dir := filepath.Dir(path)
	for i := 0; i < 5; i++ {
		if strings.Contains(dir, "xqs-plugin-") || strings.Contains(dir, "xqs-github-stage-") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Dir(path)
}

func (s *GitHubPluginService) loadReleaseChecksums(ctx context.Context, owner, repo string, release *infragithub.Release) map[string]string {
	if release == nil {
		return nil
	}
	for _, asset := range release.Assets {
		if asset.Name != "SHA256SUMS" && asset.Name != "checksums.txt" {
			continue
		}
		path, cleanup, err := s.downloadBinary(ctx, owner, repo, release.TagName, asset.Name, "")
		if err != nil {
			continue
		}
		data, readErr := os.ReadFile(path)
		cleanup()
		if readErr != nil {
			continue
		}
		return infragithub.ParseChecksumsFile(string(data))
	}
	return nil
}

func extractPlatformsFromRelease(assets []infragithub.Asset, checksums map[string]string) []domainplugin.PlatformInfo {
	var platforms []domainplugin.PlatformInfo
	for _, asset := range assets {
		if asset.Name == "SHA256SUMS" || asset.Name == "checksums.txt" {
			continue
		}
		osName, arch := parseGitHubAssetName(asset.Name)
		if osName == "" || arch == "" {
			continue
		}
		platforms = append(platforms, domainplugin.PlatformInfo{
			OS:        osName,
			Arch:      arch,
			AssetName: asset.Name,
			Checksum:  checksums[asset.Name],
		})
	}
	return platforms
}
