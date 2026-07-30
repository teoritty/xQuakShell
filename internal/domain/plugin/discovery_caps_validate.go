package plugin

import (
	"fmt"
	"path/filepath"
	"strings"
)

// maxDiscoveryIcons bounds contributions.discoveryIcons per plugin (ADR-014). It is a manifest
// sanity limit, not the on-disk byte-size limit (64 KiB/asset, 1 MiB total) — that check needs
// the bundle on disk and runs in infra at install time alongside the other on-disk checks.
const maxDiscoveryIcons = 64

// MaxDiscoveryIconBytes and MaxDiscoveryIconTotalBytes are the ADR-014 on-disk icon budgets. They
// are declared here, with the rest of the manifest contract, but enforced in infra: the numbers are
// part of what a plugin author must satisfy, while checking them means reading files.
const (
	MaxDiscoveryIconBytes      int64 = 64 * 1024
	MaxDiscoveryIconTotalBytes int64 = 1024 * 1024
)

// allowedDiscoveryIconExt reports whether ext (as returned by filepath.Ext, including the
// leading dot) is an accepted discovery icon format. Restricted to raster/vector formats the
// frontend renders via <img src="data:...;base64,...">; anything else (e.g. .html, .js) would
// have no legitimate reason to appear here and every reason to be refused.
func allowedDiscoveryIconExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".svg", ".png", ".ico":
		return true
	default:
		return false
	}
}

// validateDiscoveryCaps validates capabilities.discovery and contributions.discoveryIcons
// (ADR-014), mirroring the style of validateChannelCaps: nil capability short-circuits, then
// each declared field is checked in turn.
func (m *Manifest) validateDiscoveryCaps() error {
	if err := validateDiscoveryParentProtocols(m.Capabilities.Discovery); err != nil {
		return err
	}
	return m.validateDiscoveryIcons()
}

func validateDiscoveryParentProtocols(d *DiscoveryCaps) error {
	if d == nil {
		return nil
	}
	if len(d.ParentProtocols) == 0 {
		return fmt.Errorf("%w: discovery.parentProtocols must not be empty", ErrInvalidManifest)
	}
	for _, p := range d.ParentProtocols {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("%w: discovery.parentProtocols contains an empty entry", ErrInvalidManifest)
		}
	}
	return nil
}

func (m *Manifest) validateDiscoveryIcons() error {
	icons := m.Contributions.DiscoveryIcons
	if len(icons) == 0 {
		return nil
	}
	// Icons declared without the capability are meaningless: nothing would ever reference
	// them, since a manifest without capabilities.discovery cannot publish nodes at all.
	if m.Capabilities.Discovery == nil {
		return fmt.Errorf("%w: contributions.discoveryIcons requires capabilities.discovery", ErrInvalidManifest)
	}
	if len(icons) > maxDiscoveryIcons {
		return fmt.Errorf("%w: contributions.discoveryIcons exceeds %d entries", ErrInvalidManifest, maxDiscoveryIcons)
	}
	seenIDs := make(map[string]struct{}, len(icons))
	for _, icon := range icons {
		id := strings.TrimSpace(icon.ID)
		if id == "" {
			return fmt.Errorf("%w: discoveryIcons entry has an empty id", ErrInvalidManifest)
		}
		if _, dup := seenIDs[id]; dup {
			return fmt.Errorf("%w: duplicate discoveryIcons id %q", ErrInvalidManifest, id)
		}
		seenIDs[id] = struct{}{}
		// ValidateViewAssetEntry treats "" as "use the default ui/index.html" (a view-contribution
		// convenience that makes no sense for an icon), so an empty asset must be rejected here,
		// before it reaches that call, with a message that names the actual problem.
		if strings.TrimSpace(icon.Asset) == "" {
			return fmt.Errorf("%w: discoveryIcons id %q: asset is required", ErrInvalidManifest, id)
		}
		// Reuse the existing asset-path validator rather than writing a second one: both
		// checks exist to keep a manifest-declared path from escaping the bundle's ui/ tree.
		if err := ValidateViewAssetEntry(icon.Asset); err != nil {
			return err
		}
		if !allowedDiscoveryIconExt(filepath.Ext(icon.Asset)) {
			return fmt.Errorf("%w: discoveryIcons id %q has unsupported asset extension %q", ErrInvalidManifest, id, filepath.Ext(icon.Asset))
		}
	}
	return nil
}
