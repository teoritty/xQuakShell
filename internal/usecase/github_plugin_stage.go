package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
)

// requireBundleForUIPlugin refuses a release that offers this platform only a bare binary for a
// plugin that ships ui/ assets.
//
// The refusal is the point. A binary install of such a plugin succeeds, registers, starts, and
// then answers every request for its own interface with a 404 — a failure that surfaces far from
// its cause and reads as a broken plugin rather than an incomplete release. Refusing before
// anything is downloaded turns that into one sentence naming the asset the publisher has to add.
func requireBundleForUIPlugin(manifest domainplugin.Manifest, platform domainplugin.PlatformInfo, tag string) error {
	if !manifest.DeclaresUIAssets() || platform.Kind == domainplugin.ReleaseAssetBundle {
		return nil
	}
	return fmt.Errorf("%w: %s ships %s, but release %s offers %s/%s only the bare binary %q; the publisher must add a %s bundle asset, or install one from a file",
		domainplugin.ErrUIPluginRequiresBundle, manifest.ID, strings.Join(manifest.DeclaredUIAssets(), ", "),
		tag, platform.OS, platform.Arch, platform.AssetName, domainplugin.BundleAssetSuffix)
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

	if err := requireBundleForUIPlugin(metadata.Manifest, *platformInfo, metadata.LatestRelease); err != nil {
		return "", func() {}, err
	}

	owner, repoName, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return "", func() {}, err
	}

	asset, assetCleanup, err := s.downloadAsset(ctx, domainplugin.AssetDownloadRequest{
		Owner:            owner,
		Repo:             repoName,
		Tag:              metadata.LatestRelease,
		AssetName:        platformInfo.AssetName,
		ExpectedChecksum: platformInfo.Checksum,
		EntryName:        metadata.Manifest.Engine.Entry,
	})
	if err != nil {
		return "", func() {}, err
	}
	defer assetCleanup()

	staged, stageCleanup, err := s.stager(asset, metadata.Manifest)
	if err != nil {
		return "", func() {}, err
	}

	if err := domainplugin.VerifyStagedIdentity(metadata.Manifest, staged.Manifest); err != nil {
		stageCleanup()
		return "", func() {}, err
	}
	if !domainplugin.StagedVersionMatchesRepo(metadata.Manifest, staged.Manifest) {
		slog.Warn("release bundle version differs from the repository manifest",
			"component", "plugin.github", "plugin", staged.Manifest.ID,
			"repoVersion", metadata.Manifest.Version, "bundleVersion", staged.Manifest.Version,
			"tag", metadata.LatestRelease)
	}

	installTag := releaseTag
	if installTag == "" {
		installTag = metadata.LatestRelease
	}
	if s.installMetaWriter != nil {
		if err := s.installMetaWriter.Write(staged.Dir, domainplugin.PluginInstallMeta{
			Source:        domainplugin.InstallMetaSourceGitHub,
			RepositoryURL: normalizedURL,
			ReleaseTag:    installTag,
		}); err != nil {
			stageCleanup()
			return "", func() {}, err
		}
	}

	return staged.Dir, stageCleanup, nil
}
