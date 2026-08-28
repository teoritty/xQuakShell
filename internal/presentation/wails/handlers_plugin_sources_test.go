package wails

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// An internal test, unlike the other handler tests in test/unit/wails: the locked-vault case needs
// AppAPI.vaultRepo set, and that field is unexported. Reaching it is the whole point - a handler
// that refuses on a locked vault cannot be shown to refuse from outside the package.
//
// The marketplace catalog is stubbed here rather than imported from infra. Presentation may not
// import internal/infra and a _test.go file in this layer is no exception, which is the rule
// working as intended: what these tests check is the vault gate and the DTO mapping, and neither
// needs the real adapter. That the shipped stub refuses is asserted where it lives, in
// test/unit/plugin/plugin_catalog_service_test.go.

type unavailableCatalog struct{}

func (unavailableCatalog) Kind() domainplugin.SourceKind { return domainplugin.SourceKindMarketplace }

func (unavailableCatalog) Available(_ context.Context) error {
	return fmt.Errorf("%w: nothing is serving plugins yet", domainplugin.ErrSourceUnavailable)
}

func (unavailableCatalog) List(_ context.Context, _ domainplugin.PluginSource) ([]domainplugin.GitHubPluginMetadata, error) {
	return nil, domainplugin.ErrSourceUnavailable
}

func (unavailableCatalog) Install(_ context.Context, _ domainplugin.CatalogInstallRequest) error {
	return domainplugin.ErrSourceUnavailable
}

type stubVaultRepo struct{ unlocked bool }

func (s *stubVaultRepo) Exists() bool                                           { return true }
func (s *stubVaultRepo) Create(_ context.Context, _ string) error               { return nil }
func (s *stubVaultRepo) Unlock(_ context.Context, _ string) error               { return nil }
func (s *stubVaultRepo) VerifyMasterPassword(_ context.Context, _ string) error { return nil }
func (s *stubVaultRepo) Lock()                                                  { s.unlocked = false }
func (s *stubVaultRepo) IsUnlocked() bool                                       { return s.unlocked }
func (s *stubVaultRepo) GetData() (*domain.VaultData, error)                    { return nil, nil }
func (s *stubVaultRepo) UpdateData(_ context.Context, _ func(*domain.VaultData) error) error {
	return nil
}

func newSourcesTestAPI(unlocked bool, catalog *usecase.PluginCatalogService) *AppAPI {
	api := &AppAPI{vaultRepo: &stubVaultRepo{unlocked: unlocked}}
	api.SetContext(context.Background())
	api.SetPluginCatalog(catalog)
	return api
}

func TestListPluginSources_RefusesOnLockedVault(t *testing.T) {
	catalog := usecase.NewPluginCatalogService(nil, unavailableCatalog{})
	api := newSourcesTestAPI(false, catalog)

	sources, err := api.ListPluginSources()
	if err == nil {
		t.Fatal("a locked vault must refuse: the source list names repositories the user registered")
	}
	if sources != nil {
		t.Errorf("got %d sources alongside the error; a refusal must not also return data", len(sources))
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("err = %q; the message must say the vault is locked so the UI can prompt", err)
	}
}

func TestListPluginSources_RefusesWithoutACatalog(t *testing.T) {
	api := newSourcesTestAPI(true, nil)

	if _, err := api.ListPluginSources(); err == nil {
		t.Error("an unwired catalog must report unavailable rather than returning an empty list")
	}
}

func TestListPluginSources_UnlockedServesTheMarketplaceRow(t *testing.T) {
	catalog := usecase.NewPluginCatalogService(nil, unavailableCatalog{})
	api := newSourcesTestAPI(true, catalog)

	sources, err := api.ListPluginSources()
	if err != nil {
		t.Fatalf("ListPluginSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want the marketplace row alone with no repositories registered", len(sources))
	}

	market := sources[0]
	if market.Kind != string(domainplugin.SourceKindMarketplace) {
		t.Errorf("kind = %q, want marketplace", market.Kind)
	}
	if market.Available {
		t.Error("the marketplace must cross the boundary as unavailable while nothing serves it")
	}
	if market.UnavailableReason == "" {
		t.Error("the DTO must carry the reason: the UI renders this string and has no fallback")
	}
	// A zero AddedAt must not cross as a year-1 timestamp, which the UI would render as a date.
	if market.AddedAt != "" {
		t.Errorf("addedAt = %q; the built-in row has no registration date and must omit it", market.AddedAt)
	}
}
