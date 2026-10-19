package synctrust

import (
	"strings"

	"xquakshell/internal/domain"
)

// What a person sees of a synced item is its name, never its record id or natural key: those are
// UUIDs and selectors, and showing them is what made a conflict read as
// `connection 0922d087-… · users.[id=u-f577…].passAuth.passwordId`. The labels below are names only;
// the frontend supplies the words around them in the person's language.

// Internal reports a record type the engine keeps for itself. A person never created one, cannot
// act on one in the trash, and would only be confused to find a resolved conflict there.
func Internal(t RecordType) bool {
	switch t {
	case TypeVaultMeta, TypeConflict, TypeContact, TypeCredentialBinding, TypeTeam:
		return true
	}
	return false
}

// nameFields is where each type keeps the name a person gave the item, as a path into its data.
var nameFields = map[RecordType][]string{
	TypeConnection:    {"name"},
	TypeFolder:        {"name"},
	TypeIdentity:      {"identity", "comment"},
	TypePassword:      {"label"},
	TypePluginSecret:  {"key"},
	TypePeerTrust:     {"subject"},
	TypePluginInstall: {"id"},
	TypePluginData:    {"path"},
}

// Describe returns the name an item's data carries, or "" when its type has none - a settings
// section is named by its type alone. It never fails: a label is for display, and data that does
// not decode simply has no name to show.
func Describe(t RecordType, data []byte) string {
	if t == TypeKnownHost {
		if v, err := decodeData[knownHostData](data); err == nil {
			if f := strings.Fields(v.Line); len(f) > 0 && !strings.HasPrefix(f[0], "@") {
				return f[0]
			} else if len(f) > 1 {
				return f[1]
			}
		}
		return ""
	}
	path, ok := nameFields[t]
	if !ok {
		return ""
	}
	obj, err := decodeObject(data)
	if err != nil {
		return ""
	}
	var cur any = obj
	for _, seg := range path {
		m, isObj := cur.(map[string]any)
		if !isObj {
			return ""
		}
		cur = m[seg]
	}
	s, _ := cur.(string)
	return s
}

// DescribeItem names the item a record type and natural key point at in the vault as it is now,
// or "" when the vault holds no such item any more.
func DescribeItem(d *domain.VaultData, t RecordType, itemKey string) string {
	m, _, err := mapperFor(t)
	if err != nil {
		return ""
	}
	data, err := itemData(m, d, itemKey)
	if err != nil {
		return ""
	}
	return Describe(t, data)
}

// elementNameFields are the fields that name an element of an id-keyed array, in order of
// preference: a connection user by its login, a jump hop or forward rule by its host.
var elementNameFields = []string{"username", "name", "label", "host", "comment"}

// DescribePath resolves the "[id=…]" segments of a conflict's field path against the item as it is
// now. The result has one entry per segment: the element's name for a selector, "" for a plain
// field name, which the frontend translates by itself. A selector whose element is gone keeps "".
func DescribePath(d *domain.VaultData, t RecordType, itemKey string, path []string) []string {
	out := make([]string, len(path))
	m, _, err := mapperFor(t)
	if err != nil {
		return out
	}
	data, err := itemData(m, d, itemKey)
	if err != nil {
		return out
	}
	obj, err := decodeObject(data)
	if err != nil {
		return out
	}
	var cur any = obj
	for i, seg := range path {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[seg]
		case []any:
			idx, err := indexByID(node, seg)
			if err != nil {
				return out
			}
			cur = node[idx]
			out[i] = elementName(cur)
		default:
			return out
		}
	}
	return out
}

func elementName(v any) string {
	obj, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, f := range elementNameFields {
		if s, ok := obj[f].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
