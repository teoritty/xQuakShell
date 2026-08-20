package plugin

import "strings"

// I18nCaps declares the i18n capability: the plugin wants to be told which language the interface
// is in, so the words it hands back are in that language.
//
// It carries no privilege at all. The host sends a language tag and nothing else — no vault
// content, no session data — which is why it needs no install-time consent of its own. It exists as
// a grant rather than as an unconditional broadcast so a plugin that does not care is not woken on
// every language change, and so the manifest states plainly which plugins claim to be translated.
//
// It is separate from `ui` on purpose: a plugin with no surface of its own still supplies words the
// user reads — discovery node labels, action captions, confirmation text — and should not have to
// claim the right to draw in order to learn what language to write in.
type I18nCaps struct {
	// Locales lists the language tags the plugin ships translations for, newest-style tags as they
	// appear in the host's own packs ("en", "ru", "pt-BR"). It is informational: the host neither
	// validates the list against what the plugin actually does nor withholds the notification for a
	// language absent from it, because only the plugin knows what its fallback looks like.
	//
	// An empty list is the honest answer for a plugin that translates itself at runtime from a
	// source the host cannot see, and must not be read as "supports nothing".
	Locales []string `json:"locales,omitempty"`
}

// SupportsLocale reports whether the plugin lists this language.
//
// Nil-safe: most plugins declare no i18n block at all, and every caller here reaches for this
// without checking first.
func (c *I18nCaps) SupportsLocale(code string) bool {
	if c == nil {
		return false
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	for _, declared := range c.Locales {
		if strings.EqualFold(strings.TrimSpace(declared), code) {
			return true
		}
	}
	return false
}
