package plugin

import (
	"strings"
	"testing"
)

func TestKeyValueParseAcceptsAJSONObject(t *testing.T) {
	pairs, err := ParseKeyValue(`{"env":"prod","tier":"web"}`)
	if err != nil {
		t.Fatalf("ParseKeyValue: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %v", pairs)
	}
}

// Order is the plugin's, not the map's. A labels editor that reshuffles its rows every time the
// panel repaints is unusable, and Go map iteration is deliberately unordered.
func TestKeyValueParsePreservesDeclarationOrder(t *testing.T) {
	pairs, err := ParseKeyValue(`{"zulu":"1","alpha":"2","mike":"3"}`)
	if err != nil {
		t.Fatalf("ParseKeyValue: %v", err)
	}
	got := []string{pairs[0].Key, pairs[1].Key, pairs[2].Key}
	want := []string{"zulu", "alpha", "mike"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key order = %v, want %v", got, want)
		}
	}
}

func TestKeyValueEmptyValueIsAnEmptyMap(t *testing.T) {
	pairs, err := ParseKeyValue("")
	if err != nil || len(pairs) != 0 {
		t.Fatalf("ParseKeyValue(\"\") = %v, %v", pairs, err)
	}
}

func TestValidateKeyValueRejectsNonObject(t *testing.T) {
	for _, raw := range []string{`["a","b"]`, `"a"`, `5`, `{`} {
		if err := ValidateKeyValueValue(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestValidateKeyValueRejectsEmptyKey(t *testing.T) {
	if err := ValidateKeyValueValue(`{"":"v"}`); err == nil {
		t.Fatal("expected an empty key to be rejected")
	}
}

// A key or value carrying a control character or a bidirectional override would be rendered next
// to fields the user trusts — the same reason ADR-014 strips them from node labels.
func TestValidateKeyValueRejectsControlCharacters(t *testing.T) {
	if err := ValidateKeyValueValue("{\"a\\u0007\":\"v\"}"); err == nil {
		t.Fatal("expected a control character in a key to be rejected")
	}
	if err := ValidateKeyValueValue("{\"a\":\"v\\u202e\"}"); err == nil {
		t.Fatal("expected a bidi override in a value to be rejected")
	}
}

func TestValidateKeyValueRejectsTooManyPairs(t *testing.T) {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i <= MaxKeyValuePairs; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k`)
		b.WriteString(strings.Repeat("x", i%3))
		b.WriteString(itoa(i))
		b.WriteString(`":"v"`)
	}
	b.WriteString("}")
	if err := ValidateKeyValueValue(b.String()); err == nil {
		t.Fatalf("expected more than %d pairs to be rejected", MaxKeyValuePairs)
	}
}

func TestValidateCodeRejectsOversizeContent(t *testing.T) {
	if err := ValidateCodeValue(strings.Repeat("x", MaxCodeFieldBytes+1)); err == nil {
		t.Fatalf("expected content over %d bytes to be rejected", MaxCodeFieldBytes)
	}
	if err := ValidateCodeValue(strings.Repeat("x", MaxCodeFieldBytes)); err != nil {
		t.Fatalf("content at exactly the limit must pass: %v", err)
	}
}

// A code block is where a plugin puts a whole inspect payload, so control characters are permitted
// there — tabs and newlines are the point — but the bidirectional overrides are not: they reorder
// what is displayed rather than adding to it.
func TestValidateCodeAllowsWhitespaceButRefusesBidiOverrides(t *testing.T) {
	if err := ValidateCodeValue("line one\n\tindented\r\n"); err != nil {
		t.Fatalf("whitespace must be allowed in a code block: %v", err)
	}
	if err := ValidateCodeValue("safe‮special"); err == nil {
		t.Fatal("expected a bidi override to be rejected")
	}
}

// Both new types are manifest-declarable, so the shared type checker must know them.
func TestNewFieldTypesAreValidTypes(t *testing.T) {
	if !isValidFieldType(FieldTypeKeyValue) || !isValidFieldType(FieldTypeCode) {
		t.Fatal("keyValue and code must be valid field types")
	}
}

// Neither may be secret: a dialog has no connection and no vault to put a secret in, so a "secret"
// one would be a plaintext string with a lock icon on it (ADR-015 §2).
func TestNewFieldTypesRejectSecret(t *testing.T) {
	for _, ft := range []FieldType{FieldTypeKeyValue, FieldTypeCode} {
		if FieldTypeAllowsSecret(ft) {
			t.Fatalf("%s must not be allowed to be secret", ft)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
