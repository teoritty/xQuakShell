package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"xquakshell/internal/domain/discovery"
)

// DiscoveryPublish is the decoded discovery.publish payload: a snapshot that fully replaces
// NodeID's children. There are no deltas by design (ADR-014 alternative 2) — a per-node snapshot
// removes a whole class of desync bug, and the 500-child cap keeps one cheap.
type DiscoveryPublish struct {
	SessionID string                `json:"sessionId"`
	NodeID    string                `json:"nodeId"`
	State     discovery.BranchState `json:"state"`
	Error     string                `json:"error,omitempty"`
	Children  []discovery.Node      `json:"children,omitempty"`
}

// DecodeDiscoveryPublish parses a discovery.publish params blob.
func DecodeDiscoveryPublish(params json.RawMessage) (DiscoveryPublish, error) {
	var payload DiscoveryPublish
	if err := json.Unmarshal(params, &payload); err != nil {
		return DiscoveryPublish{}, fmt.Errorf("discovery: decode publish: %w", err)
	}
	return payload, nil
}

// DiscoveryPublishRouter turns one inbound snapshot into the state change and the notifications it
// implies. It owns no state of its own; it is the order in which the four things that do get
// touched, which is why it is worth a file rather than a few lines inside the facade.
type DiscoveryPublishRouter struct {
	store    *DiscoveryStore
	observer *DiscoveryObserver
	leader   DiscoveryLeaderLookup
	pace     *DiscoveryPace
	icons    DiscoveryIconLookup
}

// NewDiscoveryPublishRouter creates a publish router. icons may be nil, which leaves iconIds
// unchecked — acceptable only where no manifest exists at all, never in production wiring.
func NewDiscoveryPublishRouter(store *DiscoveryStore, observer *DiscoveryObserver, leader DiscoveryLeaderLookup, pace *DiscoveryPace, icons DiscoveryIconLookup) *DiscoveryPublishRouter {
	return &DiscoveryPublishRouter{store: store, observer: observer, leader: leader, pace: pace, icons: icons}
}

// Apply routes one snapshot.
//
// Every way a publish can be turned away here is silent and returns nil:
//   - its sessionId is not the one the host is currently addressing for any connection — the
//     plugin is racing a handover or a teardown;
//   - it exceeds the rate limit — dropped, logged, plugin left alone;
//   - it names a node nobody is watching — the user collapsed the branch mid-enumeration.
//
// None of these is a protocol violation. A snapshot arriving for a session that no longer leads is
// indistinguishable from one for a session that never existed, and the host has no way to tell a
// race from garbage: the former leader is erased the moment it stands down, by design, because
// keeping a roster of recently-closed sessions to sharpen an error message would be state with no
// other purpose. ADR-014 treats expand/publish races as normal, so neither case is punished.
//
// This is deliberately NOT an authorization check. Refusing a session the plugin holds no binding
// for is IDOR defence, and it has already happened by the time a payload reaches here:
// PluginSessionRPCHandler runs SessionRPCAuthorizer.AuthorizeSessionRPC on the sessionId before
// dispatching discovery.publish, exactly as it does for channel.open. A second, weaker check of the
// same thing here would be one more place for the two to drift apart.
//
// A malformed or contradictory snapshot does return an error: that one the plugin author needs to
// see, and only that one.
func (r *DiscoveryPublishRouter) Apply(ctx context.Context, pluginID string, payload DiscoveryPublish) error {
	// No plugin call happens on this path. publish is a request on the wire (it must be refusable —
	// ADR-014 §data flow), but the host answers it from what it already knows: nothing downstream of
	// here is awaited, so ctx has nothing to cancel.
	_ = ctx

	connectionID, addressed := r.leadingConnectionFor(payload.SessionID)
	if !addressed {
		slog.Debug("discovery: publish from a session the host is not addressing, ignored",
			"component", "discovery", "pluginId", pluginID, "sessionId", payload.SessionID)
		return nil
	}
	if !r.pace.AllowPublish(pluginID, connectionID) {
		slog.Warn("discovery: publish rate limit exceeded, snapshot dropped",
			"component", "discovery", "pluginId", pluginID, "connectionId", connectionID,
			"limit", discovery.MaxPublishPerSecond)
		return nil
	}
	if !r.observer.IsObserved(connectionID, payload.NodeID) {
		// Debug, like the not-addressed case above and for the same reason: this is the hot path and
		// this drop is ordinary. A plugin mid-enumeration when the user collapsed a branch is behaving
		// correctly, and a Warn per snapshot would pace a log flood by the user's clicking. It is
		// still logged, because "the plugin publishes and nothing appears" has no other symptom.
		slog.Debug("discovery: publish for a node nobody is observing, ignored",
			"component", "discovery", "pluginId", pluginID, "connectionId", connectionID,
			"nodeId", logfmtToken(payload.NodeID))
		return nil
	}

	// Icons are reconciled against the manifest here, before the store sees the nodes: the store is
	// plugin-blind by design, and an undeclared icon must never be written and then corrected.
	children := stripUndeclaredIcons(r.icons, pluginID, payload.Children)

	removed, err := r.store.ApplySnapshot(connectionID, pluginID, payload.NodeID, payload.State, payload.Error, children)
	if err != nil {
		// Warn, and here rather than only on the wire. Every ordinary drop above returns nil, so
		// nothing reaches this line but a snapshot the host judged malformed — and a refused snapshot
		// has no other symptom than a branch that never appears. The error is still returned: the
		// plugin author is the one who can fix it.
		slog.Warn("discovery: snapshot refused", "component", "discovery", "pluginId", pluginID,
			"connectionId", connectionID, "nodeId", logfmtToken(payload.NodeID), "err", err)
		return err
	}
	// Order matters: the observed set must shed the vanished IDs before the frontend is told the
	// tree changed, so the observe that goes out alongside the redraw describes the same tree.
	if len(removed) > 0 {
		r.observer.Retain(connectionID, removed)
	}
	r.pace.Emit(connectionID, payload.NodeID)
	return nil
}

// ForgetConnection drops the pace state a connection accumulated. Called from teardown, not from
// the publish path.
func (r *DiscoveryPublishRouter) ForgetConnection(connectionID string) {
	r.pace.ForgetConnection(connectionID)
}

// ForgetPlugin drops a stopped plugin's publish budget everywhere, not merely on the connections
// where it managed to draw something.
//
// The distinction is the whole reason this is not folded into the per-connection teardown loop: a
// publish dropped because the branch was collapsed, or because the session had stopped leading,
// still spent budget and still left a window behind, while creating no tree at all. Iterating the
// trees would forget exactly the windows that had a tree to be found by, and keep the ones nothing
// else will ever reach.
func (r *DiscoveryPublishRouter) ForgetPlugin(pluginID string) {
	r.pace.ForgetPlugin(pluginID)
}

// EmitConnectionChanged announces that a connection's tree changed for a reason no publish caused —
// a plugin stopping or crashing. The node is the connection root: what changed is a whole subtree,
// not one branch.
func (r *DiscoveryPublishRouter) EmitConnectionChanged(connectionID string) {
	r.pace.Emit(connectionID, "")
}

// leadingConnectionFor resolves a publish's sessionId to the connection it may write to, and only
// then: the session must both belong to a tracked connection and currently be leading it. The two
// conditions are asked together because a caller has no use for the difference — a session that is
// tracked but not leading may write exactly as much as one that was never heard of.
func (r *DiscoveryPublishRouter) leadingConnectionFor(sessionID string) (string, bool) {
	connectionID, known := r.leader.ConnectionForSession(sessionID)
	if !known {
		return "", false
	}
	leadingSessionID, _, live := r.leader.Leading(connectionID)
	if !live || leadingSessionID != sessionID {
		return "", false
	}
	return connectionID, true
}
