package plugin

import "time"

// SourceKind names the sort of place plugins can be listed and installed from.
//
// It sits one level above Forge, and the two are not interchangeable. A Forge is a git host: it
// has repositories, tags, releases, and files attached to those releases, and every step of the
// install path - reading xqsp.json at a ref, matching an asset name to a platform, checking it
// against SHA256SUMS - is written against that shape. A registry has none of it. Asking "which
// REST dialect do I speak" is a Forge question; asking "can this place give me a listing at all"
// is a SourceKind question, and collapsing them would put marketplace branches inside code whose
// every line assumes a release asset exists.
type SourceKind string

const (
	SourceKindForge       SourceKind = "forge"
	SourceKindMarketplace SourceKind = "marketplace"
)

// MarketplaceSourceID identifies the first-party plugin registry.
//
// It is an identity, not an address to fetch: nothing is served there yet, and the marketplace
// catalog refuses every call until something is. It is a constant so the row the user sees and
// the adapter that will one day answer for it cannot disagree about which source they mean.
const MarketplaceSourceID = "https://api.xquakshell.ru"

// PluginSource is one row in the user's list of places plugins come from.
//
// Forge sources are the registered repositories, projected here rather than stored again -
// github_repos.json stays the single record of what the user added. The marketplace contributes
// one built-in row instead, which is why Removable exists: every forge row can be deleted and
// that one cannot.
type PluginSource struct {
	ID          string     `json:"id"`
	Kind        SourceKind `json:"kind"`
	DisplayName string     `json:"displayName"`
	Trusted     bool       `json:"trusted"`
	Removable   bool       `json:"removable"`
	// Available is false when the source cannot serve a listing at all, as opposed to serving an
	// empty one. The UI needs the distinction: an unavailable source shows why and disables its
	// buttons, where an empty available source is just a repository with nothing published yet.
	Available         bool       `json:"available"`
	UnavailableReason string     `json:"unavailableReason,omitempty"`
	AddedAt           time.Time  `json:"addedAt"`
	LastFetchedAt     *time.Time `json:"lastFetchedAt,omitempty"`
}

// SourceFromRepository projects a registered repository onto the source list.
func SourceFromRepository(repo GitHubRepository) PluginSource {
	name := repo.DisplayName
	if name == "" {
		name = repo.Owner + "/" + repo.Repo
	}
	return PluginSource{
		ID:            repo.URL,
		Kind:          SourceKindForge,
		DisplayName:   name,
		Trusted:       repo.Trusted,
		Removable:     true,
		Available:     true,
		AddedAt:       repo.AddedAt,
		LastFetchedAt: repo.LastFetchedAt,
	}
}

// MarketplaceSource builds the built-in registry row. A non-empty reason marks it unavailable and
// is shown to the user verbatim, so callers pass something that explains the state rather than
// naming an error type.
func MarketplaceSource(reason string) PluginSource {
	return PluginSource{
		ID:                MarketplaceSourceID,
		Kind:              SourceKindMarketplace,
		DisplayName:       "xQuakShell Marketplace",
		Trusted:           true,
		Removable:         false,
		Available:         reason == "",
		UnavailableReason: reason,
	}
}
