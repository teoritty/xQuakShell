package discovery

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrInvalidNode indicates a discovery.publish snapshot violates the node contract (ADR-014).
// Wrapped with %w together with which node and which field failed, so the message an author
// sees names the exact node ID rather than just "invalid node" — this is read by a plugin
// developer debugging their own publish, not by an end user.
var ErrInvalidNode = errors.New("invalid discovery node")

// ValidatePublish checks one discovery.publish snapshot: every node in children against the
// Node contract, plus cross-node uniqueness within the snapshot. parentID is accepted for a
// future cross-check against the publish envelope; validation here is purely structural and
// does not consult anything outside children.
//
// It deliberately does NOT enforce MaxChildrenPerPublish (see TruncateChildren — exceeding it
// is truncation, a policy decision for the calling usecase layer, not a validation failure)
// and does NOT check IconID against the manifest's declared icons (the domain package has no
// knowledge of manifests; that check belongs to the capability layer).
//
// ValidatePublish does not mutate or return children: it measures Label/Tooltip length against
// the SanitizeText'd form (so a string that is only too long because of stripped control
// characters is accepted), but the original, unsanitized bytes are still sitting in the Node
// the caller holds. A publish that passes this check is NOT yet safe to store or render —
// callers MUST run every accepted node through SanitizeNode before doing anything with it
// beyond validation.
func ValidatePublish(parentID string, children []Node) error {
	_ = parentID // reserved for a future envelope cross-check; not used by structural validation
	seenIDs := make(map[string]struct{}, len(children))
	for _, node := range children {
		if err := validateNode(node); err != nil {
			return err
		}
		if _, dup := seenIDs[node.ID]; dup {
			return fmt.Errorf("%w: duplicate node id %q", ErrInvalidNode, node.ID)
		}
		seenIDs[node.ID] = struct{}{}
	}
	return nil
}

func validateNode(node Node) error {
	if node.ID == "" {
		return fmt.Errorf("%w: node id is required", ErrInvalidNode)
	}
	if utf8.RuneCountInString(node.ID) > MaxIDLen {
		return fmt.Errorf("%w: node %q id exceeds %d characters", ErrInvalidNode, node.ID, MaxIDLen)
	}
	if node.Kind != KindGroup && node.Kind != KindInstance {
		return fmt.Errorf("%w: node %q has invalid kind %q", ErrInvalidNode, node.ID, node.Kind)
	}
	label := SanitizeText(node.Label)
	if label == "" {
		return fmt.Errorf("%w: node %q label is required", ErrInvalidNode, node.ID)
	}
	// Limits are documented in runes (see limits.go), not bytes: a non-ASCII resource name
	// (Cyrillic, CJK, ...) is ordinary input for a remote-resource tree and must not be
	// penalized for using more bytes per character than ASCII.
	if utf8.RuneCountInString(label) > MaxLabelLen {
		return fmt.Errorf("%w: node %q label exceeds %d characters", ErrInvalidNode, node.ID, MaxLabelLen)
	}
	if err := validateStatus(node.ID, node.Status); err != nil {
		return err
	}
	return validateActions(node)
}

func validateStatus(nodeID string, status *Status) error {
	if status == nil {
		return nil
	}
	if !ValidTone(status.Tone) {
		return fmt.Errorf("%w: node %q has invalid status tone %q", ErrInvalidNode, nodeID, status.Tone)
	}
	if !ValidColor(status.Color) {
		return fmt.Errorf("%w: node %q has invalid status color %q", ErrInvalidNode, nodeID, status.Color)
	}
	if utf8.RuneCountInString(SanitizeText(status.Tooltip)) > MaxTooltipLen {
		return fmt.Errorf("%w: node %q status tooltip exceeds %d characters", ErrInvalidNode, nodeID, MaxTooltipLen)
	}
	return nil
}

func validateActions(node Node) error {
	if len(node.Actions) > MaxActionsPerNode {
		return fmt.Errorf("%w: node %q has more than %d actions", ErrInvalidNode, node.ID, MaxActionsPerNode)
	}
	seenActionIDs := make(map[string]struct{}, len(node.Actions))
	seenRoles := make(map[Role]struct{}, len(node.Actions))
	for _, action := range node.Actions {
		if action.ID == "" {
			return fmt.Errorf("%w: node %q has an action with an empty id", ErrInvalidNode, node.ID)
		}
		if utf8.RuneCountInString(action.ID) > MaxIDLen {
			return fmt.Errorf("%w: node %q action id %q exceeds %d characters", ErrInvalidNode, node.ID, action.ID, MaxIDLen)
		}
		if _, dup := seenActionIDs[action.ID]; dup {
			return fmt.Errorf("%w: node %q has duplicate action id %q", ErrInvalidNode, node.ID, action.ID)
		}
		seenActionIDs[action.ID] = struct{}{}
		// Measured after sanitizing, like Node.Label and Status.Tooltip above: the ceiling is on
		// what will actually be rendered, and a string padded to length with control characters
		// must not spend the budget it will not use.
		label := SanitizeText(action.Label)
		if label == "" {
			return fmt.Errorf("%w: node %q action %q has no label", ErrInvalidNode, node.ID, action.ID)
		}
		if utf8.RuneCountInString(label) > MaxLabelLen {
			return fmt.Errorf("%w: node %q action %q label exceeds %d characters", ErrInvalidNode, node.ID, action.ID, MaxLabelLen)
		}
		if utf8.RuneCountInString(SanitizeText(action.Confirm)) > MaxConfirmLen {
			return fmt.Errorf("%w: node %q action %q confirm exceeds %d characters", ErrInvalidNode, node.ID, action.ID, MaxConfirmLen)
		}
		if !ValidRole(action.Role) {
			return fmt.Errorf("%w: node %q action %q has unknown role %q", ErrInvalidNode, node.ID, action.ID, action.Role)
		}
		// One action per role, per node. A role is what a key resolves to, and two actions claiming
		// the same key is a question with no answer — the frontend cannot ask the user which they
		// meant while their finger is already off the key. Refused here rather than disambiguated
		// there: what a legal node looks like is a question for the domain, and a UI that quietly
		// picked one would be picking, for a destructive verb, between two things it cannot compare.
		if action.Role != RoleNone {
			if _, dup := seenRoles[action.Role]; dup {
				return fmt.Errorf("%w: node %q has more than one action with role %q", ErrInvalidNode, node.ID, action.Role)
			}
			seenRoles[action.Role] = struct{}{}
		}
	}
	if node.DefaultActionID != "" {
		if _, ok := seenActionIDs[node.DefaultActionID]; !ok {
			return fmt.Errorf("%w: node %q defaultActionId %q is not among its actions", ErrInvalidNode, node.ID, node.DefaultActionID)
		}
	}
	return nil
}
