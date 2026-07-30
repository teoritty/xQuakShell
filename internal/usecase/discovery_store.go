package usecase

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"xquakshell/internal/domain/discovery"
)

// ErrDiscoveryDuplicateNode reports a snapshot that would move an existing node under a new
// parent. The whole snapshot is refused: accepting the rest would leave the tree in a state
// neither the plugin nor the host believes in.
var ErrDiscoveryDuplicateNode = errors.New("discovery: node already exists under a different parent")

// DiscoveryStore is the single owner of every connection's discovery tree state.
//
// It is keyed by connectionID, never sessionID. A connection shows one subtree no matter how many
// tabs are open, and the session that happens to carry the plugin traffic can change under it
// (ADR-014 "Leading session") — keying by session would delete a tree the user is still looking at
// the moment the leading tab closes. discovery_leader.go is the only place the two are mapped.
//
// Concurrency: one mutex, held for the whole of every method, ADR-009 discipline. Nothing in this
// file calls a plugin — every method here is pure state manipulation, and the notifications that a
// state change implies are issued by the caller after this returns. That separation is the reason
// a publish storm cannot deadlock against an outbound observe.
type DiscoveryStore struct {
	mu    sync.Mutex
	conns map[string]map[string]*discoveryTree
}

// NewDiscoveryStore creates an empty store.
func NewDiscoveryStore() *DiscoveryStore {
	return &DiscoveryStore{conns: make(map[string]map[string]*discoveryTree)}
}

// ApplySnapshot replaces parentID's children with the snapshot a plugin published, returning the
// node IDs that disappeared as a result.
//
// The removed list is not a convenience: the observed set is expressed in node IDs, so a node that
// vanished must fall out of it before the next observe is sent, or the host would keep asking a
// plugin to watch something neither side has any more. The caller (discovery_publish.go) feeds
// this list straight back into the observer.
//
// The order of operations is fixed and load-bearing:
//  1. validate — a malformed snapshot changes nothing;
//  2. refuse a node that already lives under a different parent — checked before any write, so the
//     tree is untouched when the snapshot is refused;
//  3. truncate — per-publish cap, depth cap, then the per-plugin node budget;
//  4. sanitize — ValidatePublish inspects the sanitized form but returns the raw nodes, so a
//     validated node is NOT yet safe to store (see discovery.SanitizeNode);
//  5. write.
//
// A publish for a parent this plugin has never published (or that has since vanished) is dropped
// silently with no error: it is an ordinary race against the user collapsing a branch, not a fault.
func (s *DiscoveryStore) ApplySnapshot(connID, pluginID, parentID string, state discovery.BranchState, errMsg string, children []discovery.Node) ([]string, error) {
	state = hostAcceptedBranchState(pluginID, parentID, state)
	if err := discovery.ValidatePublish(parentID, children); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tree := s.treeLocked(connID, pluginID)
	if _, known := tree.nodes[parentID]; parentID != "" && !known {
		return nil, nil
	}
	for _, child := range children {
		if existing, ok := tree.nodes[child.ID]; ok && existing.ParentID != parentID {
			return nil, fmt.Errorf("%w: node %q is under %q, not %q",
				ErrDiscoveryDuplicateNode, child.ID, existing.ParentID, parentID)
		}
	}

	admitted, truncated := s.admitLocked(tree, parentID, children)
	removed := s.removeVanishedLocked(tree, parentID, admitted)
	s.writeLocked(tree, parentID, admitted, discovery.Branch{
		State:     state,
		Error:     discovery.SanitizeText(errMsg),
		Truncated: truncated,
	})
	return removed, nil
}

// hostAcceptedBranchState maps a plugin-published branch state onto one the host will store.
//
// A plugin may only speak to what it observed: loading/ready/error. "stale" describes a host-side
// leading-session handover the plugin cannot know about, so it is dropped — silently, per ADR-014:
// it is plugin-author carelessness, not an attack, and refusing an otherwise-valid snapshot over
// one stray host-only field would throw away children that are real and still worth showing. The
// substitute is "ready" because the host did receive and apply a snapshot of children, which is
// precisely what ready means. The log line is for the plugin author, not the user.
func hostAcceptedBranchState(pluginID, parentID string, state discovery.BranchState) discovery.BranchState {
	if discovery.PluginMayPublishState(state) {
		return state
	}
	slog.Warn("discovery: dropping host-only branch state from plugin publish",
		"component", "discovery", "pluginId", pluginID, "nodeId", parentID, "state", string(state))
	return discovery.BranchReady
}

// admitLocked applies all three truncation rules and returns the children that survive plus the
// Truncated record describing what was cut. Truncation, never refusal: a user who sees 500 of 900
// containers and a "partial" marker is better served than one who sees an empty branch.
func (s *DiscoveryStore) admitLocked(tree *discoveryTree, parentID string, children []discovery.Node) ([]discovery.Node, *discovery.Truncated) {
	total := len(children)
	// All children of one publish sit at the same depth, so the depth rule is all-or-nothing.
	if tree.depthOf(parentID)+1 > discovery.MaxDepth {
		return nil, &discovery.Truncated{Shown: 0, Total: total}
	}

	kept, truncated := discovery.TruncateChildren(children)

	// The per-plugin budget counts what will remain after this snapshot's own removals: a plugin
	// republishing a full branch must not be blocked by the nodes it is about to replace.
	budget := discovery.MaxNodesPerPlugin - (len(tree.nodes) - len(s.vanishingLocked(tree, parentID, kept)))
	admitted := make([]discovery.Node, 0, len(kept))
	for _, child := range kept {
		if _, exists := tree.nodes[child.ID]; !exists {
			if budget <= 0 {
				break
			}
			budget--
		}
		admitted = append(admitted, child)
	}
	if len(admitted) < len(kept) {
		if truncated == nil {
			truncated = &discovery.Truncated{Total: total}
		}
		truncated.Shown = len(admitted)
	}
	return admitted, truncated
}

// vanishingLocked returns the node IDs that this snapshot removes: every old child of parentID
// that the snapshot no longer lists, together with that child's whole subtree.
func (s *DiscoveryStore) vanishingLocked(tree *discoveryTree, parentID string, keeping []discovery.Node) []string {
	surviving := make(map[string]struct{}, len(keeping))
	for _, child := range keeping {
		surviving[child.ID] = struct{}{}
	}
	var vanished []string
	for _, oldID := range tree.children[parentID] {
		if _, stays := surviving[oldID]; stays {
			continue
		}
		vanished = append(vanished, tree.subtree(oldID)...)
	}
	return vanished
}

func (s *DiscoveryStore) removeVanishedLocked(tree *discoveryTree, parentID string, keeping []discovery.Node) []string {
	removed := s.vanishingLocked(tree, parentID, keeping)
	for _, id := range removed {
		delete(tree.nodes, id)
		delete(tree.children, id)
		delete(tree.branches, id)
	}
	return removed
}

func (s *DiscoveryStore) writeLocked(tree *discoveryTree, parentID string, admitted []discovery.Node, branch discovery.Branch) {
	ids := make([]string, 0, len(admitted))
	for _, child := range admitted {
		node := discovery.SanitizeNode(child)
		// The publish envelope is authoritative for parentage. The tree is indexed by parent, so a
		// child claiming some other parent would file itself under a branch nobody published,
		// unreachable and immune to the next snapshot that should have replaced it.
		node.ParentID = parentID
		tree.nodes[node.ID] = node
		ids = append(ids, node.ID)
	}
	tree.sortChildIDs(ids)
	tree.children[parentID] = ids
	tree.branches[parentID] = branch
}

// SetBranchState is the host's own door onto Branch.State, used for transitions no plugin caused
// (stale on handover, error on a failed one). It never touches Truncated or Error, which describe
// a specific past publish and stay true across a state change.
//
// It is deliberately not a general setter for plugin-reported state: everything a plugin says
// about a branch arrives through ApplySnapshot, where PluginMayPublishState guards it.
func (s *DiscoveryStore) SetBranchState(connID, pluginID, nodeID string, state discovery.BranchState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tree, ok := s.conns[connID][pluginID]
	if !ok {
		return
	}
	branch, ok := tree.branches[nodeID]
	if !ok {
		return
	}
	branch.State = state
	tree.branches[nodeID] = branch
}

// MarkConnectionStale flags every branch of a connection as stale, the host-side transition that
// covers a leading-session handover: the tree on screen is the old leader's answer and nothing has
// re-confirmed it yet. The nodes themselves are left untouched — they are the plugin's data, and
// the host has no standing to rewrite them just because the transport moved.
func (s *DiscoveryStore) MarkConnectionStale(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tree := range s.conns[connID] {
		for nodeID, branch := range tree.branches {
			branch.State = discovery.BranchStale
			tree.branches[nodeID] = branch
		}
	}
}

// ClearConnection deletes a connection's whole tree. Nothing is cached or persisted (ADR-014
// alternative 4): discovery reflects remote reality, and a tree with no authenticated transport
// behind it can no longer be checked against that reality.
func (s *DiscoveryStore) ClearConnection(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, connID)
}

// ClearPlugin deletes one plugin's slice of a connection's tree, leaving other plugins' subtrees
// standing — the teardown a single crashed or disabled plugin warrants.
func (s *DiscoveryStore) ClearPlugin(connID, pluginID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trees, ok := s.conns[connID]
	if !ok {
		return
	}
	delete(trees, pluginID)
	if len(trees) == 0 {
		delete(s.conns, connID)
	}
}

func (s *DiscoveryStore) treeLocked(connID, pluginID string) *discoveryTree {
	trees, ok := s.conns[connID]
	if !ok {
		trees = make(map[string]*discoveryTree)
		s.conns[connID] = trees
	}
	tree, ok := trees[pluginID]
	if !ok {
		tree = newDiscoveryTree()
		trees[pluginID] = tree
	}
	return tree
}
