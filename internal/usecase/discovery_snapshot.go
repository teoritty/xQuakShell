package usecase

import (
	"sort"

	"xquakshell/internal/domain/discovery"
)

// DiscoveryNodeView is one node as a reader needs it: the plugin's own node plus the icon the host
// resolved for it. Icon is computed here rather than by the frontend because resolution walks the
// node's ancestors (discovery.ResolveIcon) and only the store holds that chain.
type DiscoveryNodeView struct {
	Node discovery.Node
	Icon string
}

// DiscoveryPluginTreeView is one plugin's slice of a connection's tree. The read model keeps
// plugins separate for the same reason the store does: node IDs are plugin-chosen and two plugins
// may pick the same one, so a single flat list keyed by node ID would collide.
//
// Nodes are ordered parents-first so a reader can build the tree in one pass. Branches is keyed by
// node ID, with "" standing for the connection root.
type DiscoveryPluginTreeView struct {
	PluginID string
	Nodes    []DiscoveryNodeView
	Branches map[string]discovery.Branch
}

// DiscoverySnapshot is one connection's whole discovery tree, addressed the only way anything
// outside the backend is addressed: by connectionID. No sessionID appears here or anywhere
// downstream of it (ADR-014).
type DiscoverySnapshot struct {
	ConnectionID string
	Plugins      []DiscoveryPluginTreeView
}

// Snapshot returns a deep copy of a connection's tree for reading.
//
// It copies rather than exposing the live maps because the reader is on another goroutine — the
// frontend emit path — and the store keeps mutating under publishes. Handing out the internal maps
// would make every reader a second owner of state that is supposed to have exactly one.
func (s *DiscoveryStore) Snapshot(connID string) DiscoverySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := DiscoverySnapshot{ConnectionID: connID}
	pluginIDs := make([]string, 0, len(s.conns[connID]))
	for pluginID := range s.conns[connID] {
		pluginIDs = append(pluginIDs, pluginID)
	}
	sort.Strings(pluginIDs)

	for _, pluginID := range pluginIDs {
		tree := s.conns[connID][pluginID]
		snapshot.Plugins = append(snapshot.Plugins, DiscoveryPluginTreeView{
			PluginID: pluginID,
			Nodes:    tree.viewNodes(),
			Branches: tree.viewBranches(),
		})
	}
	return snapshot
}

// viewNodes walks the tree from the connection root in pre-order, so every node appears after its
// parent and siblings keep the order sortChildIDs assigned.
func (t *discoveryTree) viewNodes() []DiscoveryNodeView {
	views := make([]DiscoveryNodeView, 0, len(t.nodes))
	var walk func(parentID string)
	walk = func(parentID string) {
		for _, id := range t.children[parentID] {
			node, ok := t.nodes[id]
			if !ok {
				continue
			}
			views = append(views, DiscoveryNodeView{Node: node, Icon: discovery.ResolveIcon(node, t.ancestors(node))})
			walk(id)
		}
	}
	walk("")
	return views
}

func (t *discoveryTree) viewBranches() map[string]discovery.Branch {
	branches := make(map[string]discovery.Branch, len(t.branches))
	for nodeID, branch := range t.branches {
		// Truncated is a pointer inside Branch, so a shallow map copy would still share it with
		// the store and let a reader mutate live state through it.
		if branch.Truncated != nil {
			truncated := *branch.Truncated
			branch.Truncated = &truncated
		}
		branches[nodeID] = branch
	}
	return branches
}
