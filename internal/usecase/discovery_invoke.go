package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
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
	audit  domainplugin.DiscoveryAuditRecorder
}

// NewDiscoveryInvoker creates an action invoker. audit may be nil only where no audit log exists;
// production wiring always supplies one, because the audit entry is the sole lasting record of what
// a mass action was aimed at.
func NewDiscoveryInvoker(store *DiscoveryStore, leader DiscoveryLeaderLookup, caller DiscoveryCaller, audit domainplugin.DiscoveryAuditRecorder) *DiscoveryInvoker {
	return &DiscoveryInvoker{store: store, leader: leader, caller: caller, audit: audit}
}

// record writes one audit entry for a named phase. The node list is copied: the caller's slice came
// from the presentation layer and the recorder may hold on to the entry.
func (i *DiscoveryInvoker) record(phase, connectionID, pluginID, sessionID, actionID string, nodeIDs []string, err error) {
	if i.audit == nil {
		return
	}
	entry := domainplugin.DiscoveryAuditEntry{
		Timestamp:    time.Now(),
		Action:       phase,
		PluginID:     pluginID,
		ConnectionID: connectionID,
		SessionID:    sessionID,
		NodeIDs:      append([]string(nil), nodeIDs...),
		ActionID:     actionID,
		Success:      err == nil,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	i.audit(entry)
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
// pluginID is required, not inferred. Node IDs are plugin-chosen and each plugin gets its own tree,
// so two plugins may legitimately publish a node called "containers" under one connection; picking
// the owner by searching the trees would resolve such a collision by map iteration order, which in
// Go is randomized — a destructive action would then reach the wrong plugin some fraction of the
// time. The caller always knows whose subtree was clicked, because Snapshot is grouped by plugin,
// so the addressee is stated rather than guessed.
func (i *DiscoveryInvoker) InvokeAction(ctx context.Context, connectionID, pluginID string, nodeIDs []string, actionID string) error {
	if len(nodeIDs) == 0 || len(nodeIDs) > discovery.MaxNodesPerInvoke {
		err := fmt.Errorf("%w: %d (allowed 1..%d)", ErrDiscoveryInvokeSize, len(nodeIDs), discovery.MaxNodesPerInvoke)
		i.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, "", actionID, nodeIDs, err)
		return err
	}
	if err := i.store.CheckAction(connectionID, pluginID, nodeIDs, actionID); err != nil {
		i.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, "", actionID, nodeIDs, err)
		return err
	}
	sessionID, _, ok := i.leader.Leading(connectionID)
	if !ok {
		err := fmt.Errorf("%w: %s", ErrDiscoveryNoLeadingSession, connectionID)
		i.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, "", actionID, nodeIDs, err)
		return err
	}
	params, err := json.Marshal(discoveryInvokePayload{SessionID: sessionID, NodeIDs: nodeIDs, ActionID: actionID})
	if err != nil {
		err = fmt.Errorf("discovery: encode invokeAction: %w", err)
		i.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, sessionID, actionID, nodeIDs, err)
		return err
	}
	// The full node list is recorded, not just the count: a mass action's blast radius is exactly
	// which nodes it hit (ADR-014 "Security model"). The durable record is the audit entry below;
	// this log line is its debugging counterpart and is not a substitute for it.
	slog.Info("discovery: invoking action", "component", "discovery", "pluginId", pluginID,
		"connectionId", connectionID, "actionId", logfmtToken(actionID), "nodeIds", logfmtTokens(nodeIDs))
	// Audited before the call, not after: an action that reached the plugin and then timed out has
	// still been dispatched, and an entry written only on success would omit exactly the invocations
	// an incident review is looking for. The outcome is appended as a second entry.
	i.record(domainplugin.DiscoveryAuditDispatch, connectionID, pluginID, sessionID, actionID, nodeIDs, nil)
	if _, err := i.caller.CallWithTimeout(ctx, pluginID, discoveryInvokeMethod, params, discovery.InvokeAckTimeout); err != nil {
		err = fmt.Errorf("discovery: invokeAction: %w", err)
		i.record(domainplugin.DiscoveryAuditResult, connectionID, pluginID, sessionID, actionID, nodeIDs, err)
		return err
	}
	return nil
}

// CheckAction verifies an action is legitimately invocable on a selection of one named plugin's
// nodes, without contacting anybody.
//
// It lives on the store because every question it asks — do these nodes exist, do they share a
// parent, is the branch above them healthy — is a question about the tree, and the tree has one
// owner.
func (s *DiscoveryStore) CheckAction(connID, pluginID string, nodeIDs []string, actionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tree, ok := s.conns[connID][pluginID]
	// One missing node refuses the whole call. A selection is a set the user saw on screen; if part
	// of it is already gone, the tree they acted on is not the tree that exists, and guessing which
	// half they still meant is not the host's call to make.
	if !ok || !tree.hasAll(nodeIDs) {
		// The ids go through logfmtToken, not straight into %v. Every error built in this file ends
		// up in the audit line's detail= field, and node ids are plugin-authored — an unfiltered
		// `x result=allowed` here forges a pair in a field that never touched an id column. See
		// logfmtToken. Counting them as well as naming them keeps the message useful when a long
		// selection has been reduced to tokens.
		return fmt.Errorf("%w: plugin %q, %d nodes [%s]", ErrDiscoveryNodeNotFound,
			logfmtToken(pluginID), len(nodeIDs), strings.Join(logfmtTokens(nodeIDs), " "))
	}
	return tree.checkAction(nodeIDs, actionID)
}

func (t *discoveryTree) checkAction(nodeIDs []string, actionID string) error {
	parentID := t.nodes[nodeIDs[0]].ParentID
	multi := len(nodeIDs) > 1
	for _, nodeID := range nodeIDs {
		node := t.nodes[nodeID]
		if node.ParentID != parentID {
			// Selection is limited to siblings (ADR-014 "Actions"): an action shared by nodes from
			// unrelated branches means different things in each, and the core cannot tell.
			return fmt.Errorf("%w: %q is not a sibling of %q", ErrDiscoveryMixedParents,
				logfmtToken(nodeID), logfmtToken(nodeIDs[0]))
		}
		action, ok := findDiscoveryAction(node, actionID)
		if !ok {
			return fmt.Errorf("%w: node %q has no action %q", ErrDiscoveryActionUnavailable,
				logfmtToken(nodeID), logfmtToken(actionID))
		}
		if multi && !action.Multi {
			return fmt.Errorf("%w: action %q on node %q is not multi", ErrDiscoveryActionUnavailable,
				logfmtToken(actionID), logfmtToken(nodeID))
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
			return fmt.Errorf("%w: %q", ErrDiscoveryBranchNotActionable, logfmtToken(current))
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
