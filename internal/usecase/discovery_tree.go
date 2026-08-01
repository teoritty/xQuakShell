package usecase

import (
	"sort"

	"xquakshell/internal/domain/discovery"
)

// discoveryTree is one plugin's slice of one connection's discovery tree.
//
// Trees are per (connection, plugin) rather than one shared tree per connection because node IDs
// are plugin-chosen opaque strings (ADR-014): two plugins enumerating the same machine may both
// publish a node called "containers", and a shared index would silently let one overwrite the
// other's subtree.
//
// It carries no lock of its own. Every field here is reached only through DiscoveryStore, which
// holds the single mutex guarding all of them — the ADR-009 rule that state has exactly one
// owner. Nothing in this file may call a plugin.
type discoveryTree struct {
	nodes    map[string]discovery.Node
	children map[string][]string
	branches map[string]discovery.Branch
}

func newDiscoveryTree() *discoveryTree {
	return &discoveryTree{
		nodes:    make(map[string]discovery.Node),
		children: make(map[string][]string),
		branches: make(map[string]discovery.Branch),
	}
}

// depthOf reports how many edges separate nodeID from the connection root. The root itself ("")
// is depth 0, a node published directly under it is depth 1.
//
// The walk is bounded by MaxDepth+2 rather than run to exhaustion: depth is enforced on every
// insert, so a longer chain cannot exist, and if one ever did the caller must not spin forever
// resolving it. Returning an over-limit depth makes the caller truncate, which is exactly the
// right response to a chain that should not be there.
func (t *discoveryTree) depthOf(nodeID string) int {
	depth := 0
	for current := nodeID; current != ""; {
		node, ok := t.nodes[current]
		if !ok {
			break
		}
		depth++
		if depth > discovery.MaxDepth+1 {
			break
		}
		current = node.ParentID
	}
	return depth
}

// subtree returns nodeID together with every descendant, in no particular order. Used to compute
// what a snapshot removed: a node that vanished takes its whole subtree with it, since a
// collapsed-away grandchild has no parent left to be reached through.
func (t *discoveryTree) subtree(nodeID string) []string {
	collected := make([]string, 0, 1)
	pending := []string{nodeID}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		collected = append(collected, current)
		pending = append(pending, t.children[current]...)
	}
	return collected
}

// ancestors returns node's ancestors ordered nearest-first, the order discovery.ResolveIcon
// requires. Passing them in the other direction silently resolves the wrong icon, which is why
// the walk lives here next to the parent index rather than being rebuilt by each caller.
func (t *discoveryTree) ancestors(node discovery.Node) []discovery.Node {
	var chain []discovery.Node
	for parentID := node.ParentID; parentID != ""; {
		parent, ok := t.nodes[parentID]
		if !ok {
			break
		}
		chain = append(chain, parent)
		if len(chain) > discovery.MaxDepth+1 {
			break
		}
		parentID = parent.ParentID
	}
	return chain
}

// sortChildIDs orders one parent's children by ADR-014's rule: Order, then Label, then ID.
//
// The ADR's third key is pluginID, which cannot discriminate here — every node in one tree comes
// from the same plugin. ID stands in as the tie-breaker so the order is total and stable across
// republishes; merging several plugins' children under one parent is the presentation layer's
// job and the only place pluginID can matter.
func (t *discoveryTree) sortChildIDs(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		left, right := t.nodes[ids[i]], t.nodes[ids[j]]
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.ID < right.ID
	})
}

// hasAll reports whether every id is present in this tree. It is how an invocation finds which
// plugin owns a selection: the nodes of one mass action must all come from one tree.
func (t *discoveryTree) hasAll(ids []string) bool {
	for _, id := range ids {
		if _, ok := t.nodes[id]; !ok {
			return false
		}
	}
	return true
}
