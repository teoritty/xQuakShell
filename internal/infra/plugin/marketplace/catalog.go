// Package marketplace holds the client for the first-party plugin registry at api.xquakshell.ru.
//
// The service does not exist yet, so what lives here refuses every call. That is a deliberate
// choice over writing the HTTP client now: a client aimed at a server nobody can start is a
// client nobody can test, and the failure a user would actually see from it - a DNS error naming
// a host they have never heard of - says less than an outright refusal does. The seam is the
// Catalog port; filling it in later is a body swap, not a redesign.
package marketplace

import (
	"context"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// BaseURL is where the registry will live. Nothing dials it.
const BaseURL = domainplugin.MarketplaceSourceID

// Catalog is the registry's implementation of the plugin catalog port.
//
// The shape it is expected to speak, recorded here so the eventual implementation is written
// against a decision rather than reinventing one:
//
//	GET  /v1/plugins                       the listing
//	GET  /v1/plugins/{id}                  one plugin with its published versions
//	POST /v1/plugins/{id}/{version}/fetch  a short-lived signed URL for the asset
//
// Whatever it returns still passes through the same manifest validation, signature policy and
// consent enforcement as a forge install. The registry is a source of bytes, never a source of
// trust: being first-party is not a reason to skip a check that exists to catch a compromised
// publisher.
type Catalog struct{}

// New builds the registry catalog.
func New() *Catalog {
	return &Catalog{}
}

// Kind identifies this catalog as the marketplace one.
func (c *Catalog) Kind() domainplugin.SourceKind {
	return domainplugin.SourceKindMarketplace
}

// Available always reports the registry as unavailable.
//
// The context is unused, and named so, because there is no call to cancel: this returns a
// constant. It gains a body when there is a service to probe.
func (c *Catalog) Available(_ context.Context) error {
	return unavailable()
}

// List refuses: there is nothing to list yet.
func (c *Catalog) List(_ context.Context, _ domainplugin.PluginSource) ([]domainplugin.GitHubPluginMetadata, error) {
	return nil, unavailable()
}

// Install refuses: there is nothing to install from yet.
func (c *Catalog) Install(_ context.Context, _ domainplugin.CatalogInstallRequest) error {
	return unavailable()
}

// unavailable names the address in the message so a user who sees it in the source list knows
// which service is missing rather than only that one is.
func unavailable() error {
	return fmt.Errorf("%w: %s is not serving plugins yet", domainplugin.ErrSourceUnavailable, BaseURL)
}
