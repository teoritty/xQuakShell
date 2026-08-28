package main

import (
	domainplugin "xquakshell/internal/domain/plugin"
	inframarketplace "xquakshell/internal/infra/plugin/marketplace"
	"xquakshell/internal/usecase"
)

// Wiring the catalog router: which places this build can list and install plugins from.
//
// It lives in its own file rather than in main_plugins.go because it is its own subject, and
// because main_plugins.go is the most bug-fixed file in the repository - adding an unrelated
// concern to it buys nothing and risks something.

// buildPluginCatalog assembles the source-aware catalog from whatever is available.
//
// Both arguments may be nil: GitHub storage is best-effort at startup, and a build with no forge
// services still has a source list to draw - it just has only the marketplace row in it, which is
// a more useful screen than an error.
func buildPluginCatalog(
	repos *usecase.GitHubRepositoryService,
	plugins *usecase.GitHubPluginService,
) *usecase.PluginCatalogService {
	catalogs := []domainplugin.Catalog{inframarketplace.New()}
	if plugins != nil {
		catalogs = append(catalogs, usecase.NewForgeCatalog(plugins))
	}
	return usecase.NewPluginCatalogService(repos, catalogs...)
}
