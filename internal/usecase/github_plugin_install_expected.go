package usecase

import (
	"context"
	"strings"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ExpectedInstall installs a GitHub plugin that another of the user's devices recorded by
// reference (spec §6.4), held to the build that device recorded.
type ExpectedInstall struct {
	RepoURL string
	Tag     string
	// ManifestDigest and SumsDigest are the recorded hex SHA-256 of plugin.json and SHA256SUMS.
	ManifestDigest string
	SumsDigest     string
	Consents       domainplugin.InstallConsents
	// AcceptDifferent installs a build whose digests differ from the record, after the user was told
	// and chose to keep it as a new version.
	AcceptDifferent bool
}

// InstallExpected runs the normal install pipeline - download, mandatory checksum, signature against
// the trusted publisher keys, the user's consents - and compares the staged build with the recorded
// one before anything is written to the plugins directory. Comparing after the install would have
// let a replaced release start once before it was refused: commitInstall ends by starting it.
//
// A repository this device never added is added as an untrusted source, the state the user would
// reach by adding it by hand; trusting it stays their explicit choice.
func (s *GitHubPluginService) InstallExpected(ctx context.Context, req ExpectedInstall) error {
	normalizedURL, err := domainplugin.NormalizeURL(req.RepoURL)
	if err != nil {
		return err
	}
	if err := s.registerOfferedRepository(ctx, normalizedURL); err != nil {
		return err
	}
	tag := strings.TrimSpace(req.Tag)
	if err := s.validateReleaseTag(ctx, normalizedURL, tag); err != nil {
		return err
	}
	metadata, err := s.FetchPluginMetadataForRelease(ctx, normalizedURL, tag)
	if err != nil {
		return err
	}
	stageDir, cleanup, err := s.downloadAndStage(ctx, normalizedURL, tag, metadata)
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
	c := req.Consents
	if err := enforceInstallConsents(preview, c.SecretAccess, c.AuthProvider, c.TunnelProvider, c.MultiSession, c.ArbitraryNetwork, c.Exec); err != nil {
		return err
	}
	if !req.AcceptDifferent && (preview.ManifestDigest != req.ManifestDigest || preview.ChecksumsDigest != req.SumsDigest) {
		return ErrPluginDigestMismatch
	}
	return s.commitInstall(ctx, normalizedURL, stageDir, policy, preview, c.SecretAccess, c.AuthProvider, c.TunnelProvider, c.MultiSession, c.ArbitraryNetwork, c.Exec)
}

// registerOfferedRepository adds an offered plugin's repository as an untrusted source when this
// device does not have it yet.
func (s *GitHubPluginService) registerOfferedRepository(ctx context.Context, normalizedURL string) error {
	if repo, err := s.storage.Get(ctx, normalizedURL); err == nil && repo != nil {
		return nil
	}
	owner, name, err := domainplugin.ParseGitHubURL(normalizedURL)
	if err != nil {
		return err
	}
	repo := domainplugin.GitHubRepository{URL: normalizedURL, Owner: owner, Repo: name,
		DisplayName: owner + "/" + name, AddedAt: time.Now()}
	if err := repo.Validate(); err != nil {
		return err
	}
	return s.storage.Add(ctx, repo)
}
