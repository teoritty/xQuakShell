package synctrust

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"xquakshell/internal/domain"
)

// SetField writes value at path into one item of the vault, through the same validators as any
// synced change. It is how a person resolves a conflict in favour of the value that lost: path is
// the conflict's path, so "[id=…]" segments address elements of id-keyed arrays exactly as the merge
// named them. A write that would change the item's natural key - move it to another record - is
// refused.
func SetField(d *domain.VaultData, t RecordType, itemKey string, path []string, value json.RawMessage) error {
	if len(path) == 0 {
		return fmt.Errorf("%w: empty field path", ErrRecordData)
	}
	m, _, err := mapperFor(t)
	if err != nil {
		return err
	}
	current, err := itemData(m, d, itemKey)
	if err != nil {
		return err
	}
	obj, err := decodeObject(current)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("%w: value: %w", ErrRecordData, err)
	}
	if err := setPath(obj, path, v); err != nil {
		return err
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecordData, err)
	}
	if key, err := m.keyOf(data); err != nil || key != itemKey {
		return fmt.Errorf("%w: a field write cannot change the item's key", ErrRecordData)
	}
	mut, err := m.upsert(itemKey, data)
	if err != nil {
		return err
	}
	mut(d)
	return nil
}

func itemData(m mapper, d *domain.VaultData, itemKey string) ([]byte, error) {
	entries, err := m.entries(d)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Key == itemKey {
			return e.Data, nil
		}
	}
	return nil, fmt.Errorf("%w: no item %q", ErrRecordData, itemKey)
}

func setPath(obj map[string]any, path []string, v any) error {
	var cur any = obj
	for i, seg := range path {
		last := i == len(path)-1
		switch node := cur.(type) {
		case map[string]any:
			if last {
				node[seg] = v
				return nil
			}
			cur = node[seg]
		case []any:
			idx, err := indexByID(node, seg)
			if err != nil {
				return err
			}
			if last {
				node[idx] = v
				return nil
			}
			cur = node[idx]
		default:
			return fmt.Errorf("%w: path %v runs through a value", ErrRecordData, path[:i+1])
		}
	}
	return nil
}

// indexByID finds the element an "[id=…]" segment names.
func indexByID(list []any, seg string) (int, error) {
	id, ok := strings.CutPrefix(seg, "[id=")
	id, closed := strings.CutSuffix(id, "]")
	if !ok || !closed {
		return 0, fmt.Errorf("%w: %q is not an element of an id list", ErrRecordData, seg)
	}
	for i, el := range list {
		if obj, isObj := el.(map[string]any); isObj && obj["id"] == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: no element %s", ErrRecordData, seg)
}
