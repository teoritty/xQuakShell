package discovery

import (
	"strings"
	"unicode"
)

// bidiOverrideRunes are the Unicode bidirectional-control code points that can reorder how
// surrounding text renders. Left unfiltered in a plugin-supplied Label, one of these lets a
// resource name visually spoof its neighbor in the tree — e.g. render "evil.exe" so it reads
// as "exe.evil" — without changing a single byte an author would notice in a code review of
// the plugin, because the bytes are invisible control characters, not glyphs.
var bidiOverrideRunes = map[rune]struct{}{
	'‪': {}, // LEFT-TO-RIGHT EMBEDDING
	'‫': {}, // RIGHT-TO-LEFT EMBEDDING
	'‬': {}, // POP DIRECTIONAL FORMATTING
	'‭': {}, // LEFT-TO-RIGHT OVERRIDE
	'‮': {}, // RIGHT-TO-LEFT OVERRIDE
	'⁦': {}, // LEFT-TO-RIGHT ISOLATE
	'⁧': {}, // RIGHT-TO-LEFT ISOLATE
	'⁨': {}, // FIRST STRONG ISOLATE
	'⁩': {}, // POP DIRECTIONAL ISOLATE
}

// SanitizeText strips control characters and the bidirectional override/isolate code points
// from s. It is applied to every plugin-authored display string (Label, Tooltip) before the
// string reaches validation, storage, or rendering — see ADR-014 "Security model".
//
// Control characters are detected with unicode.IsControl, which covers BOTH the C0 set
// (U+0000-U+001F, U+007F) and the C1 set (U+0080-U+009F) in one check. C1 matters here even
// though it rarely appears in hand-typed text: U+009B is the single-byte form of the ANSI CSI
// introducer, so leaving C1 unfiltered would let a "sanitized" label still smuggle an escape
// sequence into any terminal-like surface that renders it. The ordinary space (U+0020) is not
// a control character under this definition and passes through untouched.
func SanitizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if _, isBidi := bidiOverrideRunes[r]; isBidi {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
