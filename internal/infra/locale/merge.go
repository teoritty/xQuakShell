package locale

import (
	"maps"
	"strings"

	"xquakshell/internal/domain"
)

// overlay returns base with over's messages laid on top.
//
// The one exception is the security namespace: a key under domain.SecurityMessagePrefix keeps the
// value it already had whenever the overlay is untrusted. That is what stops a pack dropped into
// <exe>/data/locales from rewriting a host key warning into a reassurance — see the constant's
// documentation for why that matters more than the convenience of translating those strings on
// disk. An untrusted overlay may still ADD a security key the base never had, which is what lets a
// language with no built-in pack be translated fully; it just cannot replace one.
func overlay(base domain.LocalePack, over domain.LocalePack, trusted bool) domain.LocalePack {
	merged := domain.LocalePack{
		Code:     base.Code,
		Name:     base.Name,
		Messages: make(map[string]string, len(base.Messages)+len(over.Messages)),
	}
	maps.Copy(merged.Messages, base.Messages)
	for key, text := range over.Messages {
		if mayOverride(merged.Messages, key, trusted) {
			merged.Messages[key] = text
		}
	}
	if over.Code != "" {
		merged.Code = over.Code
	}
	if name := strings.TrimSpace(over.Name); name != "" && (trusted || merged.Name == "") {
		merged.Name = name
	}
	return merged
}

// mayOverride reports whether an overlay is allowed to write this key.
func mayOverride(existing map[string]string, key string, trusted bool) bool {
	if trusted || !strings.HasPrefix(key, domain.SecurityMessagePrefix) {
		return true
	}
	_, alreadySet := existing[key]
	return !alreadySet
}
