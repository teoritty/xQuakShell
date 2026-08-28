package usecase

import (
	"context"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginCatalogService routes catalog work to whichever catalog serves a source's kind.
//
// Routing is all it does. Every question about how a particular source works belongs to that
// source's catalog, and the moment this file starts to branch on which one it is talking to, the
// port has failed to earn its keep.
type PluginCatalogService struct {
	repos    *GitHubRepositoryService
	catalogs map[domainplugin.SourceKind]domainplugin.Catalog
}

// NewPluginCatalogService registers one catalog per source kind. A later catalog claiming a kind
// an earlier one already took replaces it, which only happens if the composition root wires two -
// and a silent duplicate there is a wiring bug worth failing loudly on rather than a state to
// support, so the composition root passes each kind exactly once.
func NewPluginCatalogService(repos *GitHubRepositoryService, catalogs ...domainplugin.Catalog) *PluginCatalogService {
	registry := make(map[domainplugin.SourceKind]domainplugin.Catalog, len(catalogs))
	for _, catalog := range catalogs {
		if catalog == nil {
			continue
		}
		registry[catalog.Kind()] = catalog
	}
	return &PluginCatalogService{repos: repos, catalogs: registry}
}

// ListSources returns every place plugins can come from: the repositories the user registered,
// then the built-in marketplace row.
//
// The marketplace comes last on purpose. It is the one source that cannot currently serve
// anything, and a permanently unavailable row at the top of the list reads as the feature being
// broken rather than pending.
func (s *PluginCatalogService) ListSources(ctx context.Context) ([]domainplugin.PluginSource, error) {
	var sources []domainplugin.PluginSource

	if s.repos != nil {
		repos, err := s.repos.ListRepositories(ctx)
		if err != nil {
			return nil, fmt.Errorf("list plugin repositories: %w", err)
		}
		for i := range repos {
			sources = append(sources, domainplugin.SourceFromRepository(repos[i]))
		}
	}

	return append(sources, domainplugin.MarketplaceSource(s.marketplaceUnavailableReason(ctx))), nil
}

// marketplaceUnavailableReason returns the empty string when the registry can serve requests, and
// otherwise the message shown next to its row.
func (s *PluginCatalogService) marketplaceUnavailableReason(ctx context.Context) string {
	catalog, ok := s.catalogs[domainplugin.SourceKindMarketplace]
	if !ok {
		return "The plugin marketplace is not available in this build."
	}
	if err := catalog.Available(ctx); err != nil {
		return err.Error()
	}
	return ""
}

// List reads the plugins a source offers.
func (s *PluginCatalogService) List(ctx context.Context, source domainplugin.PluginSource) ([]domainplugin.GitHubPluginMetadata, error) {
	catalog, err := s.catalogFor(source.Kind)
	if err != nil {
		return nil, err
	}
	return catalog.List(ctx, source)
}

// Install installs one plugin from its source. The consents travel with the request and are
// enforced by the catalog's own install path, not here.
func (s *PluginCatalogService) Install(ctx context.Context, req domainplugin.CatalogInstallRequest) error {
	catalog, err := s.catalogFor(req.Source.Kind)
	if err != nil {
		return err
	}
	return catalog.Install(ctx, req)
}

func (s *PluginCatalogService) catalogFor(kind domainplugin.SourceKind) (domainplugin.Catalog, error) {
	catalog, ok := s.catalogs[kind]
	if !ok {
		return nil, fmt.Errorf("%w: no catalog serves source kind %q", domainplugin.ErrSourceUnavailable, kind)
	}
	return catalog, nil
}
