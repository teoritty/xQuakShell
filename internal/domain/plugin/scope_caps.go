package plugin

// ScopeCaps declares that a plugin needs a scope: one folder whose contents the user chooses to
// expose to it (ADR-022).
//
// It carries no fields, and that is the design. The core creates, names and owns the folder; a
// plugin that could name or draw its own would draw a second one resembling the first, and personal
// connections would be dropped into it by the user's own hand - the exact outcome the scope exists
// to prevent. Declaring the need is all a plugin gets to do.
type ScopeCaps struct{}

// NeedsScope reports whether the manifest asks for a scope folder.
func (m *Manifest) NeedsScope() bool {
	return m != nil && m.Capabilities.Scope != nil
}
