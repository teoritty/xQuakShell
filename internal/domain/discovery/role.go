package discovery

// Role names what an action IS, from a closed vocabulary, so the host can bind a keyboard shortcut
// to it without knowing what the action DOES.
//
// Everything else about an action stays opaque (ADR-014 "Actions"): the core draws the label and
// relays the same plugin-chosen actionId whether the click came from the menu or from a key. A role
// answers only the one question a shortcut asks and a menu never does — which of several actions the
// key means. The alternatives were worse: deriving it from `danger` would let a "kill" action answer
// the Delete key, and deriving it from a label would make a destructive shortcut depend on a
// plugin's wording and its translation.
//
// A closed vocabulary rather than a free string for the same reason Tone is one: the host acts on
// this value, so it must be a value the host understands.
type Role string

const (
	// RoleNone is the absence of a role: an ordinary menu entry no shortcut reaches. It is the zero
	// value, so an action that says nothing gets no shortcut, which is the safe default.
	RoleNone Role = ""
	// RoleDelete is what the Delete key runs.
	RoleDelete Role = "delete"
)

// ValidRole reports whether r is a role this host understands.
//
// An unknown role REJECTS the publish rather than being blanked. The vocabulary is part of the
// plugin API contract, and a plugin naming a verb this host has never heard of is describing a tree
// this host cannot render faithfully — silently dropping the role would leave a menu entry that
// looks bound to a key and is not. A plugin that wants a newer role must require a newer host
// through requires.capabilities.discovery.min.
func ValidRole(r Role) bool {
	switch r {
	case RoleNone, RoleDelete:
		return true
	default:
		return false
	}
}
