package discovery

import "strings"

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

// SanitizeText strips control characters (everything below U+0020, plus DEL U+007F) and the
// bidirectional override/isolate code points from s. It is applied to every plugin-authored
// display string (Label, Tooltip) before the string reaches validation, storage, or rendering
// — see ADR-014 "Security model". The ordinary space (U+0020) is not a control character and
// passes through untouched.
func SanitizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if _, isBidi := bidiOverrideRunes[r]; isBidi {
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
