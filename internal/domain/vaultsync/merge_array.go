package vaultsync

import "slices"

// idList is an array whose every element is an object with a unique, non-empty string "id".
type idList struct {
	order []string
	byID  map[string]any
}

func asIDList(v any) (idList, bool) {
	arr, ok := v.([]any)
	if !ok {
		return idList{}, false
	}
	list := idList{byID: make(map[string]any, len(arr))}
	for _, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			return idList{}, false
		}
		id, ok := obj["id"].(string)
		if _, dup := list.byID[id]; !ok || id == "" || dup {
			return idList{}, false
		}
		list.order = append(list.order, id)
		list.byID[id] = obj
	}
	return list, true
}

func (l idList) get(id string) any {
	if v, ok := l.byID[id]; ok {
		return v
	}
	return Absent
}

// idArray merges two id-keyed arrays element by element. The result keeps the local order and
// appends elements only the remote side has, then any the base had that neither side ordered, so
// the order is deterministic for the one device that performs the merge.
func (m *merger) idArray(path []string, base, local, remote any) ([]any, bool) {
	l, okL := asIDList(local)
	r, okR := asIDList(remote)
	if !okL || !okR {
		return nil, false
	}
	b, okB := asIDList(base)
	if !okB {
		if base != Absent {
			return nil, false
		}
		b = idList{byID: map[string]any{}}
	}
	order := slices.Clone(l.order)
	for _, ids := range [][]string{r.order, b.order} {
		for _, id := range ids {
			if !slices.Contains(order, id) {
				order = append(order, id)
			}
		}
	}
	out := []any{}
	for _, id := range order {
		v := m.value(childPath(path, "[id="+id+"]"), b.get(id), l.get(id), r.get(id))
		if v != Absent {
			out = append(out, v)
		}
	}
	return out, true
}
