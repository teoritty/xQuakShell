package plugin

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// KeyValuePair is one row of a keyValue field.
type KeyValuePair struct {
	Key   string
	Value string
}

// ParseKeyValue decodes a keyValue field's stored value: a JSON object, in declaration order.
//
// Order is preserved deliberately. The obvious implementation — unmarshal into a map — loses it,
// and Go randomises map iteration, so a labels editor would reshuffle its rows on every repaint.
// That is why this walks the token stream instead: the order is part of what the plugin said.
func ParseKeyValue(raw string) ([]KeyValuePair, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("keyValue: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("keyValue: value must be a JSON object")
	}

	var pairs []KeyValuePair
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("keyValue: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("keyValue: object key must be a string")
		}
		valTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("keyValue: %w", err)
		}
		value, ok := valTok.(string)
		if !ok {
			// Numbers and booleans are refused rather than coerced: a driver option written as 5
			// and one written as "5" reach the far end differently, and guessing which the plugin
			// meant is exactly the kind of helpfulness that produces an unreproducible bug.
			return nil, fmt.Errorf("keyValue: value for key %q must be a string", key)
		}
		pairs = append(pairs, KeyValuePair{Key: key, Value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("keyValue: %w", err)
	}
	return pairs, nil
}

// ValidateKeyValueValue checks a keyValue field's value.
//
// Keys and values are display strings that land beside fields the user trusts, so the ADR-014 rule
// applies here too: control characters and bidirectional overrides are refused rather than
// stripped, because unlike a tree label — which the host sanitizes and shows anyway — this value
// is about to be sent somewhere as data, and silently altering data is worse than refusing it.
func ValidateKeyValueValue(raw string) error {
	pairs, err := ParseKeyValue(raw)
	if err != nil {
		return err
	}
	if len(pairs) > MaxKeyValuePairs {
		return fmt.Errorf("keyValue: %d pairs exceeds the limit of %d", len(pairs), MaxKeyValuePairs)
	}
	seen := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		if strings.TrimSpace(pair.Key) == "" {
			return fmt.Errorf("keyValue: key must not be empty")
		}
		if _, dup := seen[pair.Key]; dup {
			return fmt.Errorf("keyValue: duplicate key %q", pair.Key)
		}
		seen[pair.Key] = struct{}{}
		if err := refuseUnsafeRunes("key", pair.Key); err != nil {
			return err
		}
		if err := refuseUnsafeRunes("value", pair.Value); err != nil {
			return err
		}
	}
	return nil
}

// refuseUnsafeRunes rejects control characters and bidirectional overrides in a keyValue entry.
func refuseUnsafeRunes(what, s string) error {
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("keyValue: %s contains a control character", what)
		}
		if isBidiOverrideRune(r) {
			return fmt.Errorf("keyValue: %s contains a bidirectional override", what)
		}
	}
	return nil
}

// EncodeKeyValue renders pairs back to the stored form. Used by tests and by any host code that
// needs to hand a plugin a value it can round-trip.
func EncodeKeyValue(pairs []KeyValuePair) (string, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, pair := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(pair.Key)
		if err != nil {
			return "", err
		}
		value, err := json.Marshal(pair.Value)
		if err != nil {
			return "", err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.String(), nil
}
