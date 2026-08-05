package plugin

import (
	"fmt"
	"strings"
)

// validateUICaps validates capabilities.ui (ADR-015), following the shape of validateChannelCaps
// and validateDiscoveryCaps: a nil capability short-circuits, then each declared field is checked
// in turn and every refusal names the field it refused.
func (m *Manifest) validateUICaps() error {
	ui := m.Capabilities.UI
	if ui == nil {
		return nil
	}
	if err := validateUISurfaceKinds(ui); err != nil {
		return err
	}
	if err := validateUIMaxSurfaces(ui); err != nil {
		return err
	}
	if ui.NodeDetails && m.Capabilities.Discovery == nil {
		return fmt.Errorf("%w: ui.nodeDetails requires capabilities.discovery", ErrInvalidManifest)
	}
	// Checked last, so a manifest that misspelled a surface kind is told about the misspelling
	// rather than about the empty grant that followed from it.
	if !ui.GrantsAnything() {
		return fmt.Errorf("%w: capabilities.ui grants nothing", ErrInvalidManifest)
	}
	return nil
}

func validateUISurfaceKinds(ui *UICaps) error {
	seen := make(map[string]struct{}, len(ui.Surfaces))
	for _, raw := range ui.Surfaces {
		kind, err := ParseSurfaceKind(raw)
		if err != nil {
			return fmt.Errorf("%w: ui.surfaces: %v", ErrInvalidManifest, err)
		}
		if _, dup := seen[string(kind)]; dup {
			return fmt.Errorf("%w: duplicate ui.surfaces entry %q", ErrInvalidManifest, strings.TrimSpace(raw))
		}
		seen[string(kind)] = struct{}{}
	}
	return nil
}

func validateUIMaxSurfaces(ui *UICaps) error {
	if ui.MaxSurfaces == 0 {
		return nil
	}
	if ui.MaxSurfaces < 0 {
		return fmt.Errorf("%w: ui.maxSurfaces must not be negative", ErrInvalidManifest)
	}
	if ui.MaxSurfaces > MaxSurfacesPerPluginCeiling {
		return fmt.Errorf("%w: ui.maxSurfaces exceeds the host ceiling of %d", ErrInvalidManifest, MaxSurfacesPerPluginCeiling)
	}
	// A cap on something the plugin may not open at all is a declaration that cannot mean
	// anything, and the likeliest cause is a surfaces list that was meant to be there.
	if len(ui.Surfaces) == 0 {
		return fmt.Errorf("%w: ui.maxSurfaces requires a non-empty ui.surfaces list", ErrInvalidManifest)
	}
	return nil
}
