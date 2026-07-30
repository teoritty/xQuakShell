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

// TestSanitizeTextStripsC0ControlCharacters checks the spec's literal output, not the boundary
// condition the implementation happens to use internally: comparing against a fixed expected
// string means this test cannot pass merely because it re-derives whatever range the
// implementation filters.
func TestSanitizeTextStripsC0ControlCharacters(t *testing.T) {
	in := "line1\nline2\ttab\x00null\x7fdel"
	want := "line1line2tabnulldel"
	if got := discovery.SanitizeText(in); got != want {
		t.Fatalf("got %q, want %q (control characters removed with no substitution)", got, want)
	}
}

// TestSanitizeTextStripsC1ControlCharacters covers the C1 control range (U+0080-U+009F), which a
// naive "r < 0x20 || r == 0x7f" filter (the bug found in review) does not touch. U+0085 is NEL
// (Next Line); U+009B is the single-byte encoding of the ANSI CSI introducer — leaving it
// unfiltered would let a "sanitized" label still start an escape sequence on a terminal-like
// surface. Built with explicit rune codepoints rather than literal bytes so the test source
// itself stays unambiguous about which characters are under test.
func TestSanitizeTextStripsC1ControlCharacters(t *testing.T) {
	nel := string(rune(0x85)) // NEL
	csi := string(rune(0x9b)) // single-byte CSI introducer
	in := "before" + nel + "middle" + csi + "after"
	want := "beforemiddleafter"
	if got := discovery.SanitizeText(in); got != want {
		t.Fatalf("got %q, want %q (C1 control characters removed)", got, want)
	}
}

func TestSanitizeTextPreservesOrdinarySpace(t *testing.T) {
	in := "hello world"
	if got := discovery.SanitizeText(in); got != in {
		t.Fatalf("got %q, want space preserved", got)
	}
}
