package discovery_test

import (
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestSanitizeNodeCleansLabelAndTooltip(t *testing.T) {
	in := discovery.Node{
		ID:    "a",
		Label: "evil\x00name",
		Status: &discovery.Status{
			Tone:    discovery.ToneOK,
			Tooltip: "evil\x00tooltip",
		},
	}
	out := discovery.SanitizeNode(in)
	if out.Label != "evilname" {
		t.Fatalf("Label = %q, want sanitized", out.Label)
	}
	if out.Status.Tooltip != "eviltooltip" {
		t.Fatalf("Status.Tooltip = %q, want sanitized", out.Status.Tooltip)
	}
}

func TestSanitizeNodeHandlesNilStatus(t *testing.T) {
	in := discovery.Node{ID: "a", Label: "clean"}
	out := discovery.SanitizeNode(in)
	if out.Status != nil {
		t.Fatalf("expected nil Status to stay nil, got %+v", out.Status)
	}
}

// TestSanitizeNodeDoesNotAliasInputStatus proves the returned Node's Status is a distinct
// pointer, so a caller sanitizing a node concurrently used elsewhere cannot corrupt the
// original via the copy it now holds.
func TestSanitizeNodeDoesNotAliasInputStatus(t *testing.T) {
	original := &discovery.Status{Tone: discovery.ToneOK, Tooltip: "clean"}
	in := discovery.Node{ID: "a", Label: "x", Status: original}
	out := discovery.SanitizeNode(in)
	if out.Status == original {
		t.Fatal("expected SanitizeNode to return a new Status pointer, not alias the input")
	}
	if original.Tooltip != "clean" {
		t.Fatalf("expected original Status untouched, got %q", original.Tooltip)
	}
}

// Action text was the gap: SanitizeNode cleaned the node and left every menu entry alone. Confirm
// is the worst place to leave it — a right-to-left override in the sentence a user reads before a
// destructive action can make the prompt say something other than what the action does.
func TestSanitizeNodeCleansActionText(t *testing.T) {
	in := discovery.Node{
		ID:    "a",
		Label: "container-1",
		Actions: []discovery.Action{{
			ID:      "remove",
			Label:   "Remo\u202eve",
			Confirm: "Remove\x00 these?",
			Role:    discovery.RoleDelete,
		}},
	}
	out := discovery.SanitizeNode(in)
	if out.Actions[0].Label != "Remove" {
		t.Fatalf("action Label = %q, want the bidi override stripped", out.Actions[0].Label)
	}
	if out.Actions[0].Confirm != "Remove these?" {
		t.Fatalf("action Confirm = %q, want sanitized", out.Actions[0].Confirm)
	}
	// Everything that is not free text rides through untouched.
	if out.Actions[0].ID != "remove" || out.Actions[0].Role != discovery.RoleDelete {
		t.Fatalf("sanitizing must not disturb the action's identity: %+v", out.Actions[0])
	}
}

// The input belongs to the caller, who may be holding it concurrently. A slice is mutable state,
// so it has to be copied rather than written through — the same rule Status already followed.
func TestSanitizeNodeDoesNotAliasInputActions(t *testing.T) {
	in := discovery.Node{
		ID:      "a",
		Label:   "container-1",
		Actions: []discovery.Action{{ID: "remove", Label: "Remo\x00ve"}},
	}
	out := discovery.SanitizeNode(in)
	if in.Actions[0].Label != "Remo\x00ve" {
		t.Fatalf("the caller's action was mutated: %q", in.Actions[0].Label)
	}
	if &out.Actions[0] == &in.Actions[0] {
		t.Fatal("the returned node shares its action slice with the input")
	}
}

// A node with no actions must not grow an empty non-nil slice, so an untouched node round-trips
// byte-identically through the store.
func TestSanitizeNodeLeavesAnActionlessNodeAlone(t *testing.T) {
	out := discovery.SanitizeNode(discovery.Node{ID: "a", Label: "container-1"})
	if out.Actions != nil {
		t.Fatalf("Actions = %#v, want nil", out.Actions)
	}
}
