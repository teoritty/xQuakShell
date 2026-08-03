package discovery

// ResolveIcon computes the effective icon for node: its own IconID if set, otherwise the
// IconID of the nearest ancestor that set one, otherwise "".
//
// ancestors must be ordered from the nearest ancestor (node's direct parent) to the root —
// the function returns on the first match it finds walking that order, so passing the list in
// the wrong direction silently picks the wrong icon instead of failing.
//
// Inheritance is a fallback, never an override (ADR-014 "Icons"): a group that sets its own
// IconID always wins for itself and stops the walk for its own children's fallback — a
// "volumes" subgroup under "docker" with its own icon does not hand that icon down further,
// and "docker"'s icon never leaks past a subgroup that already declared its own.
func ResolveIcon(node Node, ancestors []Node) string {
	if node.IconID != "" {
		return node.IconID
	}
	for _, ancestor := range ancestors {
		if ancestor.IconID != "" {
			return ancestor.IconID
		}
	}
	return ""
}
