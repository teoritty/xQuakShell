package domain

import (
	"errors"
	"regexp"
)

// DefaultLocale is the language every other one falls back to, key by key. It is also what an
// unset or unrecognised setting resolves to, so a vault carrying a language this build has never
// heard of still opens in a language the user can read.
const DefaultLocale = "en"

// SecurityMessagePrefix marks the keys a language pack on disk may translate but must never
// override.
//
// A pack under <exe>/data/locales is written by whoever can reach that folder, and its strings go
// straight into the interface. Host key verification, plugin install consent and trust policy are
// decided by the user reading a warning, so a pack that could rewrite "the host key changed —
// this may be a machine-in-the-middle attack" into "everything is fine, press Accept" would turn
// a translation file into a way to talk a user out of the only check standing between them and a
// forged server. Keys under this prefix therefore come from the built-in packs only.
const SecurityMessagePrefix = "security."

// localeCodePattern is the whole of what a language code may look like: "en", "pt-BR". Nothing else.
//
// The code arrives from the settings vault and from the name of a file someone dropped next to the
// executable, and it is used to build a path. An allowlist this narrow is what makes that safe:
// "..", a separator, a NUL and a drive letter are all outside it, so a traversal attempt is
// rejected as a malformed code before any path is joined rather than caught afterwards by a
// containment check. The containment check still runs where the file is opened — this is the first
// of two, not a replacement for the second.
var localeCodePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

// ValidLocaleCode reports whether code is an acceptable language code.
func ValidLocaleCode(code string) bool {
	return localeCodePattern.MatchString(code)
}

// NormalizeLocaleCode returns the language to actually use for a requested one.
//
// An unrecognised code is not an error: the setting may have been written by a build that knows
// more languages than this one, and the useful behaviour when opening such a vault is to show
// English rather than to refuse.
func NormalizeLocaleCode(code string) string {
	if !ValidLocaleCode(code) {
		return DefaultLocale
	}
	return code
}

// ErrLocaleCodeInvalid indicates a language code did not match the accepted shape.
var ErrLocaleCodeInvalid = errors.New("invalid locale code")

// ErrLocaleNotFound indicates no built-in or on-disk pack carries the requested language.
var ErrLocaleNotFound = errors.New("locale not found")

// LocaleInfo describes one selectable language without carrying its messages, so the language
// list can be shown without loading every catalogue.
type LocaleInfo struct {
	Code string
	// Name is the language's own name for itself ("Русский"), not its English name. A user
	// looking for their language in a list they cannot currently read finds it by its endonym.
	Name    string
	Builtin bool
}

// LocalePack is one language's full message catalogue.
type LocalePack struct {
	Code     string
	Name     string
	Messages map[string]string
}

// LocaleCatalog serves the languages this installation can display.
//
// Pack returns a catalogue already merged with DefaultLocale, so a key missing from a translation
// arrives as its English text rather than as a blank or a raw key. Doing the fallback here rather
// than in the interface keeps one implementation of the rule, in the layer that can test it.
type LocaleCatalog interface {
	Available() []LocaleInfo
	Pack(code string) (LocalePack, error)
}
