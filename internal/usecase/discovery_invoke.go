package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"xquakshell/internal/domain/discovery"
)

// discoveryInvokeMethod is the host->plugin request carrying a (possibly mass) action.
const discoveryInvokeMethod = "discovery.invokeAction"

var (
	// ErrDiscoveryInvokeSize reports a selection outside 1..MaxNodesPerInvoke.
	ErrDiscoveryInvokeSize = errors.New("discovery: invalid number of nodes for one action")
	// ErrDiscoveryNodeNotFound reports a selection the host cannot fully resolve in the tree it
	// currently holds.
	ErrDiscoveryNodeNotFound = errors.New("discovery: node not found")
	// ErrDiscoveryMixedParents reports a selection spanning more than one parent.
	ErrDiscoveryMixedParents = errors.New("discovery: selected nodes must share one parent")
	// ErrDiscoveryActionUnavailable reports an action missing from a selected node, or one not
	// marked multi in a multi-node selection.
	ErrDiscoveryActionUnavailable = errors.New("discovery: action not available on every selected node")
	// ErrDiscoveryBranchNotActionable reports an action attempted inside a stale or failed branch.
	ErrDiscoveryBranchNotActionable = errors.New("discovery: branch is stale or failed")
	// ErrDiscoveryNoLeadingSession reports an action on a connection with no ready session.
	ErrDiscoveryNoLeadingSession = errors.New("discovery: connection has no ready session")
)

// DiscoveryCaller sends a host->plugin request with an explicit timeout. Satisfied by
// *PluginManager.
type DiscoveryCaller interface {
	CallWithTimeout(ctx context.Context, pluginID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error)
}

// DiscoveryInvoker validates an action against the tree the host actually holds, then relays it.
//
// Every check below runs before the plugin is contacted, and a failed check means no call at all.
// That is the whole design: the core has no idea what an action does (ADR-014 "Actions"), so it
// cannot undo one, and the only moment it can protect the user is before the request leaves.
type DiscoveryInvoker struct {
	store  *DiscoveryStore
	leader DiscoveryLeaderLookup
	caller DiscoveryCaller
}

// NewDiscoveryInvoker creates an action invoker.
func NewDiscoveryInvoker(store *DiscoveryStore, leader DiscoveryLeaderLookup, caller DiscoveryCaller) *DiscoveryInvoker {
	return &DiscoveryInvoker{store: store, leader: leader, caller: caller}
}

// discoveryInvokePayload is the discovery.invokeAction wire shape. nodeIds is always a list, even
// for one node: a single action is a mass action over a list of one, and a second verb for that
// would be two paths to one meaning (ADR-014 alternative 3).
type discoveryInvokePayload struct {
	SessionID string   `json:"sessionId"`
	NodeIDs   []string `json:"nodeIds"`
	ActionID  string   `json:"actionId"`
}

// InvokeAction relays an action on one or more nodes of a connection's tree.
//
// The RPC waits only for an acknowledgement, bounded by InvokeAckTimeout. A plugin doing real work
// — stopping fifty containers — must not hold the request open for it; it acknowledges receipt,
// moves the nodes to a busy status itself, and reports the outcome through an ordinary publish.
// Partial success needs no protocol of its own: some nodes end up ok and others error.
func (i *DiscoveryInvoker) InvokeAction(ctx context.Context, connectionID string, nodeIDs []string, actionID string) error {
	if len(nodeIDs) == 0 || len(nodeIDs) > discovery.MaxNodesPerInvoke {
		return fmt.Errorf("%w: %d (allowed 1..%d)", ErrDiscoveryInvokeSize, len(nodeIDs), discovery.MaxNodesPerInvoke)
	}
	pluginID, err := i.store.ResolveAction(connectionID, nodeIDs, actionID)
	if err != nil {
		return err
	}
	sessionID, _, ok := i.leader.Leading(connectionID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrDiscoveryNoLeadingSession, connectionID)
	}
	params, err := json.Marshal(discoveryInvokePayload{SessionID: sessionID, NodeIDs: nodeIDs, ActionID: actionID})
	if err != nil {
		return fmt.Errorf("discovery: encode invokeAction: %w", err)
	}
	// The full node list is recorded, not just the count: a mass action's blast radius is exactly
	// which nodes it hit (ADR-014 "Security model").
	slog.Info("discovery: invoking action", "component", "discovery", "pluginId", pluginID,
		"connectionId", connectionID, "actionId", actionID, "nodeIds", nodeIDs)
	if _, err := i.caller.CallWithTimeout(ctx, pluginID, discoveryInvokeMethod, params, discovery.InvokeAckTimeout); err != nil {
		return fmt.Errorf("discovery: invokeAction: %w", err)
	}
	return nil
}

// ResolveAction finds the plugin that owns a selection and checks the action is legitimately
// invocable on all of it, without contacting anybody.
//
// It lives on the store because every question it asks — do these nodes exist, do they share a
// parent, is the branch above them healthy — is a question about the tree, and the tree has one
// owner.
func (s *DiscoveryStore) ResolveAction(connID string, nodeIDs []string, actionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for pluginID, tree := range s.conns[connID] {
		if !tree.hasAll(nodeIDs) {
			continue
		}
		if err := tree.checkAction(nodeIDs, actionID); err != nil {
			return "", err
		}
		return pluginID, nil
	}
	// One missing node refuses the whole call. A selection is a set the user saw on screen; if part
	// of it is already gone, the tree they acted on is not the tree that exists, and guessing which
	// half they still meant is not the host's call to make.
	return "", fmt.Errorf("%w: %v", ErrDiscoveryNodeNotFound, nodeIDs)
}

func (t *discoveryTree) checkAction(nodeIDs []string, actionID string) error {
	parentID := t.nodes[nodeIDs[0]].ParentID
	multi := len(nodeIDs) > 1
	for _, nodeID := range nodeIDs {
		node := t.nodes[nodeID]
		if node.ParentID != parentID {
			// Selection is limited to siblings (ADR-014 "Actions"): an action shared by nodes from
			// unrelated branches means different things in each, and the core cannot tell.
			return fmt.Errorf("%w: %q is not a sibling of %q", ErrDiscoveryMixedParents, nodeID, nodeIDs[0])
		}
		action, ok := findDiscoveryAction(node, actionID)
		if !ok {
			return fmt.Errorf("%w: node %q has no action %q", ErrDiscoveryActionUnavailable, nodeID, actionID)
		}
		if multi && !action.Multi {
			return fmt.Errorf("%w: action %q on node %q is not multi", ErrDiscoveryActionUnavailable, actionID, nodeID)
		}
	}
	return t.checkBranchActionable(parentID)
}

// checkBranchActionable walks from the selection's own branch up to the root and refuses if any
// branch on the way is stale or failed.
//
// The whole subtree is blocked, not just the branch itself, because a stale ancestor means the
// path that led here has not been re-confirmed: the nodes still on screen may name resources that
// are gone, and an action would then be aimed at whatever now answers to that name.
func (t *discoveryTree) checkBranchActionable(parentID string) error {
	for current, steps := parentID, 0; steps <= discovery.MaxDepth+1; steps++ {
		switch t.branches[current].State {
		case discovery.BranchStale, discovery.BranchError:
			return fmt.Errorf("%w: %q", ErrDiscoveryBranchNotActionable, current)
		}
		if current == "" {
			return nil
		}
		current = t.nodes[current].ParentID
	}
	return nil
}

func findDiscoveryAction(node discovery.Node, actionID string) (discovery.Action, bool) {
	for _, action := range node.Actions {
		if action.ID == actionID {
			return action, true
		}
	}
	return discovery.Action{}, false
}
