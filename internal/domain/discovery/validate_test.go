package discovery_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func baseNode(id string) discovery.Node {
	return discovery.Node{
		ID:    id,
		Kind:  discovery.KindInstance,
		Label: "container-1",
	}
}

// baseAction is a minimally valid action: every field a plugin MUST fill is filled, so a test about
// one rule can override just that field without tripping another.
func baseAction(id string) discovery.Action {
	return discovery.Action{ID: id, Label: "Do it"}
}

func TestValidatePublishAcceptsValidSnapshot(t *testing.T) {
	children := []discovery.Node{
		baseNode("a"),
		{
			ID:    "b",
			Kind:  discovery.KindGroup,
			Label: "Volumes",
			Status: &discovery.Status{
				Tone:    discovery.ToneOK,
				Color:   "#1a2b3c",
				Tooltip: "healthy",
			},
			Actions: []discovery.Action{
				{ID: "start", Label: "Start"},
				{ID: "stop", Label: "Stop"},
			},
			DefaultActionID: "start",
		},
	}
	if err := discovery.ValidatePublish("", children); err != nil {
		t.Fatalf("expected valid snapshot, got %v", err)
	}
}

func TestValidatePublishRejectsEmptyID(t *testing.T) {
	n := baseNode("")
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for empty id, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongID(t *testing.T) {
	n := baseNode(strings.Repeat("x", discovery.MaxIDLen+1))
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long id, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxLenID(t *testing.T) {
	n := baseNode(strings.Repeat("x", discovery.MaxIDLen))
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected max-length id to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsDuplicateID(t *testing.T) {
	children := []discovery.Node{baseNode("dup"), baseNode("dup")}
	err := discovery.ValidatePublish("", children)
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidKind(t *testing.T) {
	n := baseNode("a")
	n.Kind = "bogus"
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for invalid kind, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidTone(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: "bogus"}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for invalid tone, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidColor(t *testing.T) {
	cases := []string{"123abc", "#abc", "#1a2b3c4d", "#gggggg"}
	for _, c := range cases {
		n := baseNode("a")
		n.Status = &discovery.Status{Tone: discovery.ToneOK, Color: c}
		if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
			t.Fatalf("color %q: expected ErrInvalidNode, got %v", c, err)
		}
	}
}

func TestValidatePublishAcceptsValidColor(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: discovery.ToneOK, Color: "#AaBbCc"}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected valid color to pass, got %v", err)
	}
}

func TestValidatePublishRejectsEmptyLabel(t *testing.T) {
	n := baseNode("a")
	n.Label = ""
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for empty label, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongLabel(t *testing.T) {
	n := baseNode("a")
	n.Label = strings.Repeat("x", discovery.MaxLabelLen+1)
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long label, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxLenLabel(t *testing.T) {
	n := baseNode("a")
	n.Label = strings.Repeat("x", discovery.MaxLabelLen)
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected max-length label to be valid, got %v", err)
	}
}

// TestValidatePublishCountsLabelInRunesNotBytes pins limits.go's documented unit: MaxLabelLen is
// a rune count, not a byte count. A multi-byte (Cyrillic) label of exactly MaxLabelLen characters
// is ~2x that many bytes and must still pass; len()-based counting would wrongly reject it.
func TestValidatePublishCountsLabelInRunesNotBytes(t *testing.T) {
	n := baseNode("a")
	n.Label = strings.Repeat("щ", discovery.MaxLabelLen) // 2 bytes/rune in UTF-8
	if len(n.Label) <= discovery.MaxLabelLen {
		t.Fatalf("test setup invalid: label byte length %d should exceed MaxLabelLen %d", len(n.Label), discovery.MaxLabelLen)
	}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected a label of exactly MaxLabelLen runes (over MaxLabelLen bytes) to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongTooltip(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: discovery.ToneOK, Tooltip: strings.Repeat("x", discovery.MaxTooltipLen+1)}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long tooltip, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxLenTooltip(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: discovery.ToneOK, Tooltip: strings.Repeat("x", discovery.MaxTooltipLen)}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected max-length tooltip to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsTooManyActions(t *testing.T) {
	n := baseNode("a")
	for i := 0; i < discovery.MaxActionsPerNode+1; i++ {
		n.Actions = append(n.Actions, baseAction(fmt.Sprintf("action-%d", i)))
	}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too many actions, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxActions(t *testing.T) {
	n := baseNode("a")
	for i := 0; i < discovery.MaxActionsPerNode; i++ {
		n.Actions = append(n.Actions, baseAction(fmt.Sprintf("action-%d", i)))
	}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected exactly MaxActionsPerNode actions to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsDuplicateActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{baseAction("start"), baseAction("start")}
	err := discovery.ValidatePublish("", []discovery.Node{n})
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "duplicate action") {
		t.Fatalf("expected duplicate action id error, got %v", err)
	}
}

func TestValidatePublishRejectsEmptyActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: ""}}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for empty action id, got %v", err)
	}
}

// TestValidatePublishRejectsTooLongActionID pins that MaxIDLen applies to Action.ID as well as
// Node.ID, per its own doc comment in limits.go ("bounds Node.ID and Action.ID").
func TestValidatePublishRejectsTooLongActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: strings.Repeat("x", discovery.MaxIDLen+1)}}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long action id, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxLenActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{baseAction(strings.Repeat("x", discovery.MaxIDLen))}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected max-length action id to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsUnknownDefaultActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{baseAction("start")}
	n.DefaultActionID = "stop"
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for unknown defaultActionId, got %v", err)
	}
}

// TestValidatePublishDoesNotRejectOversizeChildCount pins the ADR-014 invariant that exceeding
// MaxChildrenPerPublish is truncation policy for the caller (TruncateChildren), never a
// validation failure. Written so a future "helpful" child-count check added directly to
// ValidatePublish fails this test instead of silently changing the contract.
func TestValidatePublishDoesNotRejectOversizeChildCount(t *testing.T) {
	children := make([]discovery.Node, discovery.MaxChildrenPerPublish+1)
	for i := range children {
		children[i] = baseNode(fmt.Sprintf("node-%d", i))
	}
	if err := discovery.ValidatePublish("", children); err != nil {
		t.Fatalf("ValidatePublish must not reject a snapshot for exceeding MaxChildrenPerPublish (that is TruncateChildren's job), got %v", err)
	}
}

// A role is what a keyboard shortcut resolves to, so the host acts on the value — which means it
// has to be a value the host understands. An unknown one is refused rather than blanked: silently
// dropping it would leave a menu entry that looks bound to a key and is not.
func TestValidatePublishRejectsUnknownActionRole(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: "rename", Label: "Rename…", Role: discovery.Role("rename")}}
	err := discovery.ValidatePublish("", []discovery.Node{n})
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("expected unknown role error, got %v", err)
	}
}

// No role at all is the ordinary case: an entry the menu offers and no key reaches.
func TestValidatePublishAcceptsAnActionWithoutARole(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{baseAction("start"), {ID: "remove", Label: "Remove…", Role: discovery.RoleDelete}}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("a node with one delete action and one plain action is legal: %v", err)
	}
}

// Two actions claiming one key is a question with no answer — the user's finger is already off the
// key. Refused in the domain rather than disambiguated in the UI, which would be picking, for a
// destructive verb, between two things it cannot compare.
func TestValidatePublishRejectsTwoActionsWithOneRole(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{
		{ID: "remove", Label: "Remove…", Role: discovery.RoleDelete},
		{ID: "purge", Label: "Purge", Role: discovery.RoleDelete},
	}
	err := discovery.ValidatePublish("", []discovery.Node{n})
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "more than one action with role") {
		t.Fatalf("expected duplicate role error, got %v", err)
	}
}

// Roleless actions do not collide with each other: the uniqueness rule is about claimed keys, and
// an action claiming none claims nothing.
func TestValidatePublishAcceptsManyRolelessActions(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{baseAction("start"), baseAction("stop"), baseAction("restart")}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("actions without roles must not collide: %v", err)
	}
}

// Action.Label and Action.Confirm were unbounded and unsanitized while Node.Label and
// Status.Tooltip were both — the ceiling is measured on the sanitized form, so padding a string
// with control characters cannot buy room it will not use when rendered.
func TestValidatePublishBoundsActionText(t *testing.T) {
	tooLongLabel := baseNode("a")
	tooLongLabel.Actions = []discovery.Action{{ID: "x", Label: strings.Repeat("l", discovery.MaxLabelLen+1)}}
	if err := discovery.ValidatePublish("", []discovery.Node{tooLongLabel}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for an over-long action label, got %v", err)
	}

	tooLongConfirm := baseNode("a")
	tooLongConfirm.Actions = []discovery.Action{
		{ID: "x", Label: "Remove", Confirm: strings.Repeat("c", discovery.MaxConfirmLen+1)},
	}
	if err := discovery.ValidatePublish("", []discovery.Node{tooLongConfirm}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for an over-long confirm, got %v", err)
	}

	// At the ceiling is allowed: the limit is "no more than".
	atLimit := baseNode("a")
	atLimit.Actions = []discovery.Action{{
		ID:      "x",
		Label:   strings.Repeat("l", discovery.MaxLabelLen),
		Confirm: strings.Repeat("c", discovery.MaxConfirmLen),
	}}
	if err := discovery.ValidatePublish("", []discovery.Node{atLimit}); err != nil {
		t.Fatalf("exactly at the limit must pass: %v", err)
	}

	// Control characters do not buy length: this is over the limit only before sanitizing.
	padded := baseNode("a")
	padded.Actions = []discovery.Action{{
		ID:    "x",
		Label: strings.Repeat("\x00", 50) + strings.Repeat("l", discovery.MaxLabelLen),
	}}
	if err := discovery.ValidatePublish("", []discovery.Node{padded}); err != nil {
		t.Fatalf("the ceiling applies to the sanitized form: %v", err)
	}
}

// An action with no label is a menu entry the user cannot read, which is worse than no entry.
func TestValidatePublishRejectsAnActionWithNoLabel(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: "x", Label: "\x00\x00"}}
	err := discovery.ValidatePublish("", []discovery.Node{n})
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "no label") {
		t.Fatalf("expected a missing-label error, got %v", err)
	}
}
