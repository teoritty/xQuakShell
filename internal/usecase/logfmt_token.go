package usecase

import (
	"strings"
	"unicode"

	"xquakshell/internal/domain/discovery"
)

// logfmtToken reduces a plugin-authored string to something that cannot forge structure in a
// `key=value ...` log or audit line.
//
// Two separate problems, both real:
//
//   - Control characters and bidi overrides. A newline ends the record for any line-oriented
//     reader, so everything after it becomes a forged record rather than a forged field; the
//     bidi overrides reorder how the rest of the line renders to a human.
//   - The separators themselves. A space starts the next pair and '=' binds one, so an id of
//     `x result=allowed` inserts a pair the host never wrote. Redaction alone does not help here:
//     RedactAuditDetail hunts for secrets and leaves spaces and '=' exactly where it found them.
//
// Separators are REPLACED rather than dropped. A dropped space silently welds two words into a
// plausible-looking identifier; a visible '_' shows a reader that the plugin put something there
// that had no business being in an id.
//
// This is not a substitute for redaction. Callers writing to an audit trail run RedactAuditDetail
// as well — this function makes a value structurally safe, that one makes it safe to keep.
func logfmtToken(s string) string {
	// SanitizeText covers C0 and C1 controls (unicode.IsControl) plus the bidi override and isolate
	// code points, which is the same treatment plugin-authored labels get before rendering.
	cleaned := discovery.SanitizeText(s)
	var b strings.Builder
	b.Grow(len(cleaned))
	for _, r := range cleaned {
		switch {
		case r == '=' || r == ',':
			b.WriteRune('_')
		case unicode.IsSpace(r):
			// unicode.IsSpace, not r == ' ': NEL, LS/PS, NBSP, ogham space, MVS and the rest all
			// separate fields for a splitter that tokenizes on whitespace, and several of them are
			// invisible in a log viewer.
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// logfmtTokens applies logfmtToken to every element, for the id lists that appear as one field.
func logfmtTokens(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, logfmtToken(v))
	}
	return out
}
