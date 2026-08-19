package usecase

import (
	"context"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ForgeCatalog answers the catalog port for repositories hosted on a git forge.
//
// It is an adapter and deliberately nothing more. GitHubPluginService already implements every
// step of a forge install - metadata, release resolution, download, checksum, staging, consent
// enforcement - and it is covered by tests that were written against those signatures. Wrapping
// it costs one indirection; reworking it to take a PluginSource would rewrite a working module to
// no behavioural end.
type ForgeCatalog struct {
	plugins *GitHubPluginService
}

// NewForgeCatalog wraps a plugin service as a catalog.
func NewForgeCatalog(plugins *GitHubPluginService) *ForgeCatalog {
	return &ForgeCatalog{plugins: plugins}
}

// Kind identifies this catalog as the forge one.
func (c *ForgeCatalog) Kind() domainplugin.SourceKind {
	return domainplugin.SourceKindForge
}

// Available reports a forge source as reachable without probing it.
//
// A per-source network check here would cost one round trip per registered repository every time
// the source list is drawn, to answer a question the next List call answers anyway. A repository
// that is gone reports itself when it is read, with an error naming the repository.
func (c *ForgeCatalog) Available(_ context.Context) error {
	if c.plugins == nil {
		return domainplugin.ErrSourceUnavailable
	}
	return nil
}

// List reads the plugin published by one repository.
//
// A forge repository holds exactly one plugin - xqsp.json sits at its root - so the slice is
// there to satisfy the port, not because a second entry is possible.
func (c *ForgeCatalog) List(ctx context.Context, source domainplugin.PluginSource) ([]domainplugin.GitHubPluginMetadata, error) {
	if c.plugins == nil {
		return nil, domainplugin.ErrSourceUnavailable
	}
	metadata, err := c.plugins.FetchPluginMetadata(ctx, source.ID, false)
	if err != nil {
		return nil, err
	}
	return []domainplugin.GitHubPluginMetadata{*metadata}, nil
}

// Install installs from a forge repository at the requested release tag.
func (c *ForgeCatalog) Install(ctx context.Context, req domainplugin.CatalogInstallRequest) error {
	if c.plugins == nil {
		return domainplugin.ErrSourceUnavailable
	}
	return c.plugins.InstallPluginFromGitHub(
		ctx,
		req.Source.ID,
		req.Version,
		req.Consents.SecretAccess,
		req.Consents.AuthProvider,
		req.Consents.TunnelProvider,
		req.Consents.MultiSession,
		req.Consents.ArbitraryNetwork,
		req.Consents.Exec,
	)
}
