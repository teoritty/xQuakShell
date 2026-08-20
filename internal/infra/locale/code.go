package locale

import (
	"regexp"

	"xquakshell/internal/domain"
)

// codePattern is the whole of what a language code may look like: "en", "pt-BR". Nothing else.
//
// The code arrives from the settings vault and from the name of a file someone dropped next to the
// executable, and it is used to build a path. An allowlist this narrow is what makes that safe:
// "..", a separator, a NUL and a drive letter are all outside it, so a traversal attempt is
// rejected as a malformed code before any path is joined rather than caught afterwards by a
// containment check. The containment check still runs (see disk.go) — this is the first of two,
// not a replacement for it.
var codePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// ValidCode reports whether code is an acceptable language code.
func ValidCode(code string) bool {
	return codePattern.MatchString(code)
}

// NormalizeCode returns the code to actually use for a requested one, falling back to English.
//
// An unknown code is not an error here. The setting is written by a build that may know more
// languages than this one, and the useful behaviour when reading such a vault is to show English,
// not to refuse to start.
func NormalizeCode(code string) string {
	if !ValidCode(code) {
		return domain.DefaultLocale
	}
	return code
}
