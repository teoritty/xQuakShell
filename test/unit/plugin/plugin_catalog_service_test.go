package plugin_test

import (
	"context"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	inframarketplace "xquakshell/internal/infra/plugin/marketplace"
	"xquakshell/internal/usecase"
)

// stubCatalog records what it was asked for, so the routing tests can assert that a call reached
// one catalog and not the other rather than only that it did not error.
type stubCatalog struct {
	kind          domainplugin.SourceKind
	availableErr  error
	listedSources []string
	installedIDs  []string
}

func (c *stubCatalog) Kind() domainplugin.SourceKind { return c.kind }

func (c *stubCatalog) Available(_ context.Context) error { return c.availableErr }

func (c *stubCatalog) List(_ context.Context, source domainplugin.PluginSource) ([]domainplugin.GitHubPluginMetadata, error) {
	c.listedSources = append(c.listedSources, source.ID)
	return []domainplugin.GitHubPluginMetadata{{ID: "listed-by-" + string(c.kind)}}, nil
}

func (c *stubCatalog) Install(_ context.Context, req domainplugin.CatalogInstallRequest) error {
	c.installedIDs = append(c.installedIDs, req.Source.ID)
	return nil
}

func TestListRoutesBySourceKind(t *testing.T) {
	forge := &stubCatalog{kind: domainplugin.SourceKindForge}
	market := &stubCatalog{kind: domainplugin.SourceKindMarketplace}
	svc := usecase.NewPluginCatalogService(nil, forge, market)

	got, err := svc.List(context.Background(), domainplugin.PluginSource{
		ID:   "https://github.com/o/r",
		Kind: domainplugin.SourceKindForge,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(got) != 1 || got[0].ID != "listed-by-forge" {
		t.Errorf("got %+v; a forge source must be served by the forge catalog", got)
	}
	if len(forge.listedSources) != 1 {
		t.Errorf("forge catalog saw %d calls, want 1", len(forge.listedSources))
	}
	if len(market.listedSources) != 0 {
		t.Errorf("marketplace catalog saw %d calls; routing sent the request to both", len(market.listedSources))
	}
}

func TestInstallRoutesBySourceKind(t *testing.T) {
	forge := &stubCatalog{kind: domainplugin.SourceKindForge}
	market := &stubCatalog{kind: domainplugin.SourceKindMarketplace}
	svc := usecase.NewPluginCatalogService(nil, forge, market)

	err := svc.Install(context.Background(), domainplugin.CatalogInstallRequest{
		Source: domainplugin.PluginSource{ID: "market-entry", Kind: domainplugin.SourceKindMarketplace},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(market.installedIDs) != 1 || market.installedIDs[0] != "market-entry" {
		t.Errorf("marketplace catalog saw %v, want one install of market-entry", market.installedIDs)
	}
	if len(forge.installedIDs) != 0 {
		t.Errorf("forge catalog performed %d installs; a marketplace install must not reach it", len(forge.installedIDs))
	}
}

func TestUnroutableKindReportsSourceUnavailable(t *testing.T) {
	svc := usecase.NewPluginCatalogService(nil, &stubCatalog{kind: domainplugin.SourceKindForge})

	_, err := svc.List(context.Background(), domainplugin.PluginSource{
		ID:   domainplugin.MarketplaceSourceID,
		Kind: domainplugin.SourceKindMarketplace,
	})
	if !errors.Is(err, domainplugin.ErrSourceUnavailable) {
		t.Errorf("err = %v; a kind no catalog serves must report ErrSourceUnavailable, not a bare string", err)
	}
}

func TestSourceListAlwaysCarriesTheMarketplaceRow(t *testing.T) {
	svc := usecase.NewPluginCatalogService(nil, inframarketplace.New())

	sources, err := svc.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want the marketplace row alone when no repositories are registered", len(sources))
	}

	market := sources[0]
	if market.Kind != domainplugin.SourceKindMarketplace {
		t.Errorf("kind = %q, want marketplace", market.Kind)
	}
	if market.Available {
		t.Error("the marketplace must report itself unavailable while nothing serves it")
	}
	if market.UnavailableReason == "" {
		t.Error("an unavailable source must say why; the UI shows this string verbatim")
	}
	if market.Removable {
		t.Error("the built-in marketplace row must not be removable")
	}
}

func TestMarketplaceStubRefusesEveryCall(t *testing.T) {
	catalog := inframarketplace.New()
	ctx := context.Background()

	if err := catalog.Available(ctx); !errors.Is(err, domainplugin.ErrSourceUnavailable) {
		t.Errorf("Available = %v, want ErrSourceUnavailable", err)
	}
	if _, err := catalog.List(ctx, domainplugin.MarketplaceSource("")); !errors.Is(err, domainplugin.ErrSourceUnavailable) {
		t.Errorf("List = %v, want ErrSourceUnavailable", err)
	}
	if err := catalog.Install(ctx, domainplugin.CatalogInstallRequest{}); !errors.Is(err, domainplugin.ErrSourceUnavailable) {
		t.Errorf("Install = %v, want ErrSourceUnavailable", err)
	}
}
