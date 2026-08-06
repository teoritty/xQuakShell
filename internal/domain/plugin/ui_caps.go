package plugin

import "strings"

// UICaps declares the ui capability (ADR-015): where a plugin may draw.
//
// It carries no new privilege over the remote machine. Every byte a surface displays was already
// obtainable by the plugin through channel/exec, which has its own install-time consent; this
// capability governs only where those bytes may be shown, which is why it needs no consent prompt
// of its own — the same argument ADR-014 made for discovery.
type UICaps struct {
	// Surfaces lists the surface kinds the plugin may open: "terminal", "log". An empty list means
	// the plugin may open none, not "all".
	Surfaces []string `json:"surfaces,omitempty"`
	// Dialogs grants dialog.open and its siblings.
	Dialogs bool `json:"dialogs,omitempty"`
	// NodeDetails grants the details-panel verbs for discovery nodes. It requires
	// capabilities.discovery, since there is otherwise no node to describe.
	NodeDetails bool `json:"nodeDetails,omitempty"`
	// MaxSurfaces caps concurrently open surfaces for this plugin. 0 means the host default;
	// values above MaxSurfacesPerPluginCeiling are refused at manifest load.
	MaxSurfaces int `json:"maxSurfaces,omitempty"`
}

// AllowsSurfaceKind reports whether kind appears in the declared list. An undeclared kind is a
// capability denial, never a fallback to some other kind: the plugin asked for a tab that behaves
// one way, and giving it a tab that behaves the other way would be worse than refusing.
//
// Nil-safe, because both the gate and the use case reach for this on plugins that declare no ui
// block at all.
func (c *UICaps) AllowsSurfaceKind(kind string) bool {
	if c == nil {
		return false
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	for _, declared := range c.Surfaces {
		if strings.TrimSpace(declared) == kind {
			return true
		}
	}
	return false
}

// EffectiveMaxSurfaces returns the declared cap, or the host default when the manifest left it
// unset. A nil capability returns 0: a plugin that declared no ui block may open nothing, and
// answering with the default here would hand it a budget it never asked for.
func (c *UICaps) EffectiveMaxSurfaces() int {
	if c == nil {
		return 0
	}
	if c.MaxSurfaces <= 0 {
		return MaxSurfacesPerPluginDefault
	}
	return c.MaxSurfaces
}

// GrantsAnything reports whether the block permits at least one thing. Used by validation to
// refuse a ui block that grants nothing — see validateUICaps.
func (c *UICaps) GrantsAnything() bool {
	if c == nil {
		return false
	}
	return len(c.Surfaces) > 0 || c.Dialogs || c.NodeDetails
}
