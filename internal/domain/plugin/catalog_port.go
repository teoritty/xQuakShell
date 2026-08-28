package plugin

import (
	"context"
	"errors"
)

// ErrSourceUnavailable indicates a source cannot serve requests at all - the service behind it is
// absent, unreachable, or not yet built. It is distinct from an empty listing, which is a source
// working correctly and having nothing to offer, and callers must not collapse the two: one is a
// state to report and the other is a state to retry.
var ErrSourceUnavailable = errors.New("plugin source unavailable")

// Catalog is one place plugins can be listed and installed from.
//
// Entries are GitHubPluginMetadata rather than a catalog-neutral type of their own. The name is
// forge-flavoured and the shape is not: it is already what the install path, the Wails DTO and
// the UI all speak, and a parallel type would buy a translation layer whose entire job is
// renaming fields. Renaming the type is a sweep of its own, not a side effect of adding a second
// source.
type Catalog interface {
	// Kind reports which sort of source this catalog answers for. The router dispatches on it,
	// so exactly one catalog may claim each kind.
	Kind() SourceKind

	// Available reports whether the source can serve requests, returning ErrSourceUnavailable
	// wrapped with something a user can read when it cannot. It exists so the source list can be
	// rendered honestly without fetching a listing from every source first.
	Available(ctx context.Context) error

	List(ctx context.Context, source PluginSource) ([]GitHubPluginMetadata, error)
	Install(ctx context.Context, req CatalogInstallRequest) error
}

// CatalogInstallRequest is everything an install needs that is not already implied by the source.
type CatalogInstallRequest struct {
	Source PluginSource
	// Version is the release tag on a forge and the published version on a registry. Empty means
	// "whatever the source considers current".
	Version  string
	Consents InstallConsents
}

// InstallConsents records which of the install warnings the user accepted.
//
// It is a struct rather than six parameters because six booleans in a row is a call site nobody
// can read and an argument order nobody can get wrong twice the same way. The backend still
// enforces each one; this only carries the answers.
type InstallConsents struct {
	SecretAccess     bool
	AuthProvider     bool
	TunnelProvider   bool
	MultiSession     bool
	ArbitraryNetwork bool
	Exec             bool
}
