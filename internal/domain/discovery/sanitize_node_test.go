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
