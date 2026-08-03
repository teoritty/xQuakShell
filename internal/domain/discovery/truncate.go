package discovery

// TruncateChildren enforces MaxChildrenPerPublish on one discovery.publish snapshot. When
// children fits within the limit it is returned unchanged with a nil Truncated. Otherwise the
// first MaxChildrenPerPublish entries are kept and a Truncated record describes what was cut.
//
// This is truncation, not rejection, by design (ADR-014): a plugin that legitimately
// enumerates thousands of resources should still show the user something and let them know
// the list is partial, rather than have the whole branch fail because it went one node over
// the cap.
func TruncateChildren(children []Node) ([]Node, *Truncated) {
	total := len(children)
	if total <= MaxChildrenPerPublish {
		return children, nil
	}
	return children[:MaxChildrenPerPublish], &Truncated{Shown: MaxChildrenPerPublish, Total: total}
}
