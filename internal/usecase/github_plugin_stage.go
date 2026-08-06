package usecase

import (
	"context"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

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
