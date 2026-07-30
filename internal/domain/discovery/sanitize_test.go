package discovery_test

import (
	"strings"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestSanitizeTextLeavesOrdinaryTextUnchanged(t *testing.T) {
	in := "my-container_1 (prod)"
	if got := discovery.SanitizeText(in); got != in {
		t.Fatalf("got %q, want unchanged %q", got, in)
	}
}

func TestSanitizeTextStripsEachBidiOverride(t *testing.T) {
	bidiRunes := []rune{
		'‪', '‫', '‬', '‭', '‮',
		'⁦', '⁧', '⁨', '⁩',
	}
	for _, r := range bidiRunes {
		in := "safe" + string(r) + "name"
		got := discovery.SanitizeText(in)
		if strings.ContainsRune(got, r) {
			t.Fatalf("bidi rune %U not stripped from %q -> %q", r, in, got)
		}
		if got != "safename" {
			t.Fatalf("got %q, want \"safename\" after stripping %U", got, r)
		}
	}
}

func TestSanitizeTextStripsControlCharacters(t *testing.T) {
	in := "line1\nline2\ttab\x00null\x7fdel"
	got := discovery.SanitizeText(in)
	for _, r := range got {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control character %U survived sanitization: %q", r, got)
		}
	}
	if got != "line1line2tabnulldel" {
		t.Fatalf("got %q, want control characters removed with no substitution", got)
	}
}

func TestSanitizeTextPreservesOrdinarySpace(t *testing.T) {
	in := "hello world"
	if got := discovery.SanitizeText(in); got != in {
		t.Fatalf("got %q, want space preserved", got)
	}
}
