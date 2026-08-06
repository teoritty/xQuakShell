package discovery

// SanitizeNode returns a copy of node with every piece of plugin-authored TEXT run through
// SanitizeText: Label, Status.Tooltip if present, and each action's Label and Confirm. It exists
// because ValidatePublish only *inspects* the sanitized form to decide pass/fail (see its doc
// comment) — it never mutates or returns the nodes it is given, so a node that passes validation
// can still carry the original, unsanitized bytes (e.g. control characters padding out a string
// that is short enough only after they are stripped). Any caller that stores or renders a Node a
// plugin published — not merely validates it — MUST call SanitizeNode (or apply SanitizeText
// itself) on the copy it keeps; ValidatePublish's approval is not a guarantee that the text is
// already clean.
//
// Action text is included for the same reason node text is, and one of it matters more: Confirm is
// the sentence shown in the MODAL a user reads before a destructive action, and a right-to-left
// override there can make a prompt say something other than what the action will do. IconID is
// deliberately absent — it is an identifier reconciled against the manifest by stripUndeclaredIcons
// before the store sees it, not free text.
//
// Status is replaced with a new pointer, and Actions with a new slice, so the returned Node shares
// no mutable state with the input — callers may hold onto the original node concurrently.
func SanitizeNode(node Node) Node {
	node.Label = SanitizeText(node.Label)
	if node.Status != nil {
		status := *node.Status
		status.Tooltip = SanitizeText(status.Tooltip)
		node.Status = &status
	}
	if len(node.Actions) > 0 {
		actions := make([]Action, len(node.Actions))
		for i, action := range node.Actions {
			action.Label = SanitizeText(action.Label)
			action.Confirm = SanitizeText(action.Confirm)
			actions[i] = action
		}
		node.Actions = actions
	}
	return node
}
