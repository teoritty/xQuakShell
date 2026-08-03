package discovery

// SanitizeNode returns a copy of node with Label and, if present, Status.Tooltip run through
// SanitizeText. It exists because ValidatePublish only *inspects* the sanitized form to decide
// pass/fail (see its doc comment) — it never mutates or returns the nodes it is given, so a
// node that passes validation can still carry the original, unsanitized bytes in Label or
// Tooltip (e.g. control characters padding out a string that is short enough only after they
// are stripped). Any caller that stores or renders a Node a plugin published — not merely
// validates it — MUST call SanitizeNode (or apply SanitizeText itself) on the copy it keeps;
// ValidatePublish's approval is not a guarantee that node.Label is already clean.
//
// Status is replaced with a new pointer when non-nil so the returned Node shares no mutable
// state with the input — callers may hold onto the original node concurrently.
func SanitizeNode(node Node) Node {
	node.Label = SanitizeText(node.Label)
	if node.Status != nil {
		status := *node.Status
		status.Tooltip = SanitizeText(status.Tooltip)
		node.Status = &status
	}
	return node
}
