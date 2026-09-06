package plugin

import (
	"encoding/json"
	"fmt"
	"slices"
)

// PermissionSet is the normalized, comparable set of everything a manifest asks for (ADR-022).
//
// It exists so that consent can be keyed by what was granted rather than by a version. Re-prompting
// on every update shows the dialog often enough that people stop reading it, and they then click
// through the one release where the permissions really did grow.
//
// The tokens are sorted and deduplicated, so two manifests that ask for the same things compare
// equal however their author happened to order or repeat the entries.
type PermissionSet struct {
	tokens []string
}

// newPermissionSet normalizes the tokens it is given. It takes ownership of the slice, which every
// caller builds locally for the purpose.
func newPermissionSet(tokens []string) PermissionSet {
	slices.Sort(tokens)
	return PermissionSet{tokens: slices.Compact(tokens)}
}

// Tokens returns the permissions as sorted strings, for the consent dialog and the audit log. The
// result is a copy: the set is stored state, and a caller must not be able to edit it by writing to
// a slice it was handed.
func (p PermissionSet) Tokens() []string {
	return slices.Clone(p.tokens)
}

// IsEmpty reports whether the manifest asked for nothing at all.
func (p PermissionSet) IsEmpty() bool {
	return len(p.tokens) == 0
}

// Widens reports whether this set asks for anything the granted set did not, and names what was
// added.
//
// The names are the point. A dialog saying "permissions changed" teaches people to click through;
// one that says which permission appeared does not. Giving a permission up is deliberately not a
// widening - an update that asks for less needs no new consent.
func (p PermissionSet) Widens(granted PermissionSet) (added []string, widened bool) {
	for _, token := range p.tokens {
		if !slices.Contains(granted.tokens, token) {
			added = append(added, token)
		}
	}
	return added, len(added) > 0
}

// MarshalJSON writes the tokens as an array, using an empty array rather than null for a set that
// grants nothing: a stored null would read as "no record" where the truth is "recorded, and it is
// empty", and those two must not look alike in a grant.
func (p PermissionSet) MarshalJSON() ([]byte, error) {
	if p.tokens == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(p.tokens)
}

// UnmarshalJSON re-normalizes what it reads, so a vault edited by hand into an unsorted or
// duplicated list still compares correctly rather than re-prompting for permissions already given.
func (p *PermissionSet) UnmarshalJSON(raw []byte) error {
	var tokens []string
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return fmt.Errorf("decode permission set: %w", err)
	}
	*p = newPermissionSet(tokens)
	return nil
}
