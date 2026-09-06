package plugin

// Anchor names the basis on which a plugin is permitted to reach one object (ADR-022).
//
// Before this there was one basis - an active session the plugin owns - which is right for what it
// was built for and absent for credential work: replication runs with no session at all, and
// provisioning runs before one exists. Naming the bases makes the gate say which one it used, and
// makes "none" a value rather than the absence of a decision.
type Anchor int

const (
	// AnchorNone denies. It is the zero value on purpose: a resolver that forgets to decide refuses.
	AnchorNone Anchor = iota
	// AnchorSession is an active session the plugin owns.
	AnchorSession
	// AnchorScope is the user having placed the object in this plugin's scope, with the vault open.
	//
	// The second half is not decoration. A scope is a position in a folder tree and survives a lock
	// trivially, so an anchor that did not check would hand a plugin access after the vault closed -
	// through the mechanism added to constrain it.
	AnchorScope
)

// String names the anchor for an audit line or a denial message.
func (a Anchor) String() string {
	switch a {
	case AnchorSession:
		return "session"
	case AnchorScope:
		return "scope"
	default:
		return "none"
	}
}
