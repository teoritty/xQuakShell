package usecase

import (
	"context"
	"errors"
	"slices"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

var (
	// ErrPluginOfferNotFound is an offer for a plugin no other device recorded.
	ErrPluginOfferNotFound = errors.New("no plugin from your other devices has that id")
	// ErrPluginOfferManual is an offer for a plugin installed from a file elsewhere: there is nothing
	// to download, and the user installs it by hand on this device.
	ErrPluginOfferManual = errors.New("this plugin was installed from a file; install it manually on this device")
)

// PluginOfferInstaller is the GitHub install pipeline an offer runs through.
type PluginOfferInstaller interface {
	PreviewInstall(ctx context.Context, repoURL, releaseTag string) (GitHubPluginPreviewDTO, error)
	InstallExpected(ctx context.Context, req ExpectedInstall) error
}

// OfferInstallRequest installs a plugin another device recorded.
type OfferInstallRequest struct {
	PluginID string
	Consents domainplugin.InstallConsents
	// AcceptDifferent keeps a build that differs from the record as the new version (spec §6.4).
	AcceptDifferent bool
}

// githubOffer finds the GitHub record of an offered plugin.
func (s *PluginReferenceSync) githubOffer(pluginID string) (domain.SyncPlugin, error) {
	data, err := s.d.Vault.GetData()
	if err != nil {
		return domain.SyncPlugin{}, err
	}
	if data.Sync == nil {
		return domain.SyncPlugin{}, ErrPluginOfferNotFound
	}
	i := slices.IndexFunc(data.Sync.Plugins, func(p domain.SyncPlugin) bool { return p.ID == pluginID })
	if i < 0 {
		return domain.SyncPlugin{}, ErrPluginOfferNotFound
	}
	rec := data.Sync.Plugins[i]
	if rec.Source.Kind != domain.PluginSourceGitHub {
		return domain.SyncPlugin{}, ErrPluginOfferManual
	}
	return rec, nil
}

// githubRepoURL is the repository URL of a record. The record type admits only owner/name, so the
// URL cannot point anywhere but a GitHub repository.
func githubRepoURL(rec domain.SyncPlugin) string { return "https://github.com/" + rec.Source.Repo }

// PreviewOffer prepares the consent screen for the release another device recorded.
func (s *PluginReferenceSync) PreviewOffer(ctx context.Context, pluginID string) (GitHubPluginPreviewDTO, error) {
	rec, err := s.githubOffer(pluginID)
	if err != nil {
		return GitHubPluginPreviewDTO{}, err
	}
	return s.d.Installer.PreviewInstall(ctx, githubRepoURL(rec), rec.Source.Tag)
}

// InstallOffer installs the build another device recorded through the normal pipeline. A build
// that differs from the record is refused with ErrPluginDigestMismatch before anything is written,
// unless the request accepts it, in which case it becomes the recorded version everywhere.
func (s *PluginReferenceSync) InstallOffer(ctx context.Context, req OfferInstallRequest) error {
	rec, err := s.githubOffer(req.PluginID)
	if err != nil {
		return err
	}
	if err := s.d.Installer.InstallExpected(ctx, ExpectedInstall{RepoURL: githubRepoURL(rec), Tag: rec.Source.Tag,
		ManifestDigest: rec.ManifestDigest, SumsDigest: rec.SumsDigest, Consents: req.Consents,
		AcceptDifferent: req.AcceptDifferent}); err != nil {
		return err
	}
	if req.AcceptDifferent {
		return s.AcceptInstalled(ctx, req.PluginID)
	}
	return s.Record(ctx)
}

// KeepPlugin keeps the build installed here: a mismatching build becomes the recorded version, and
// a plugin another device uninstalled is recorded again and turned back on.
func (s *PluginReferenceSync) KeepPlugin(ctx context.Context, pluginID string) error {
	data, err := s.d.Vault.GetData()
	if err != nil {
		return err
	}
	removed := data.Sync != nil && slices.Contains(data.Sync.PluginsRemovedElsewhere, pluginID)
	if err := s.AcceptInstalled(ctx, pluginID); err != nil {
		return err
	}
	if removed && s.d.Enable != nil {
		return s.d.Enable(ctx, pluginID)
	}
	return nil
}
