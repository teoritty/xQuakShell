package plugin

// Surface states. They mirror the session states the terminal UI already understands, so a tab
// that is waiting, usable, or broken looks the same to the user whoever produced it.
const (
	SurfaceStateConnecting = "connecting"
	SurfaceStateReady      = "ready"
	SurfaceStateError      = "error"
)

// Surface is one plugin-owned tab (ADR-015).
//
// It is deliberately NOT a session. A session in this core owns a connection, an SSH client, a
// vault binding and a host-key decision; a surface owns none of those and must never appear to.
// It is a view onto work the plugin is already authorized to do on an existing session, which is
// why it carries ParentSessionID rather than being addressed by one: the session is the authority
// the surface borrows, not the surface's identity.
type Surface struct {
	// ID is host-minted, prefixed to keep it disjoint from session ids by construction.
	ID string
	// PluginID owns this surface. Every surface.* call is checked against it, and a mismatch is
	// reported as a capability denial rather than as a missing surface.
	PluginID string
	// ParentSessionID is the session whose authorization this surface rides, and whose closure
	// takes it down.
	ParentSessionID string
	// ConnectionID is resolved from the parent session at open time, for display only. The
	// frontend addresses surfaces by ID and connections by ConnectionID; it never sees a session
	// id, matching the rule ADR-014 set for discovery.
	ConnectionID string
	Kind         SurfaceKind
	// Title is plugin-supplied and sanitized before it is stored: it is drawn next to tabs the
	// user trusts, so control characters and bidirectional overrides are stripped (ADR-014
	// security model) and its length is capped.
	Title string
	// IconID refers to an already-validated discoveryIcons asset, or is empty.
	IconID string
	State  string
	// Error carries the message shown when State is SurfaceStateError.
	Error string
}

// ValidSurfaceState reports whether s is one of the three states a plugin may set.
func ValidSurfaceState(s string) bool {
	switch s {
	case SurfaceStateConnecting, SurfaceStateReady, SurfaceStateError:
		return true
	default:
		return false
	}
}
