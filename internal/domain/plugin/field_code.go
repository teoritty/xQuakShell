package plugin

import "fmt"

// bidiOverrideRunes are the Unicode directional formatting characters that reorder text around
// them. Declared here rather than imported from internal/domain/discovery because domain packages
// do not import each other; the list is short, fixed by Unicode, and duplicated deliberately.
var bidiOverrides = map[rune]struct{}{
	0x202A: {}, 0x202B: {}, 0x202C: {}, 0x202D: {}, 0x202E: {},
	0x2066: {}, 0x2067: {}, 0x2068: {}, 0x2069: {},
}

func isBidiOverrideRune(r rune) bool {
	_, ok := bidiOverrides[r]
	return ok
}

// ValidateCodeValue checks a code field's content.
//
// Control characters are ALLOWED here, unlike in every other display string: a code block is where
// a plugin puts a whole inspect payload, and tabs and newlines are the point of it. Bidirectional
// overrides are still refused — they do not add to what is displayed, they reorder it, which is
// the property that makes them a spoofing tool rather than formatting.
func ValidateCodeValue(raw string) error {
	if len(raw) > MaxCodeFieldBytes {
		return fmt.Errorf("code: content of %d bytes exceeds the limit of %d", len(raw), MaxCodeFieldBytes)
	}
	for _, r := range raw {
		if isBidiOverrideRune(r) {
			return fmt.Errorf("code: content contains a bidirectional override")
		}
	}
	return nil
}

// FieldTypeAllowsSecret reports whether a field type may carry secret: true.
//
// keyValue and code may not. A secret's whole storage story is the vault, keyed by connection and
// field id; a dialog has neither, so a "secret" field of either type would be a plaintext string
// with a lock icon on it (ADR-015 §2).
func FieldTypeAllowsSecret(t FieldType) bool {
	switch t {
	case FieldTypeKeyValue, FieldTypeCode:
		return false
	default:
		return true
	}
}
