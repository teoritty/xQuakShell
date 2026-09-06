package domain

import "testing"

// The walk is bounded as well as guarded, and this is what the bound is for.
//
// NewScopeIndex refuses a parent cycle, so through the exported API this can never happen - which is
// exactly why it needs a test from inside: an unbounded walk would be correct until the day an index
// reached PluginFor without passing the constructor, and the failure would not be a wrong answer but
// a permission check that never returns.
//
// Hitting the bound must read as "in no scope". Returning the last scope seen would turn a corrupted
// folder tree into an exposure.
func TestTheScopeWalkTerminatesOnACycleItWasNotSupposedToSee(t *testing.T) {
	index := ScopeIndex{
		parent: map[string]string{"a": "b", "b": "a"},
		roots:  map[string]string{"sync": "com.example.sync"},
	}

	pluginID, ok := index.PluginFor("a")

	if ok {
		t.Fatalf("a cycle resolved to plugin %q instead of terminating", pluginID)
	}
}
