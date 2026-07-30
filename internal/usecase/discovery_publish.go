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
}

// NewDiscoveryPublishRouter creates a publish router.
func NewDiscoveryPublishRouter(store *DiscoveryStore, observer *DiscoveryObserver, leader DiscoveryLeaderLookup, pace *DiscoveryPace) *DiscoveryPublishRouter {
	return &DiscoveryPublishRouter{store: store, observer: observer, leader: leader, pace: pace}
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
// for is IDOR defence and belongs to the capability layer, where it is decided from the binding
// itself. A second, weaker check of the same thing here would be one more place for the two to
// drift apart.
//
// A malformed or contradictory snapshot does return an error: that one the plugin author needs to
// see, and only that one.
func (r *DiscoveryPublishRouter) Apply(ctx context.Context, pluginID string, payload DiscoveryPublish) error {
	_ = ctx // no plugin call happens on this path: a publish is a notification, nothing is awaited

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
		return nil
	}

	removed, err := r.store.ApplySnapshot(connectionID, pluginID, payload.NodeID, payload.State, payload.Error, payload.Children)
	if err != nil {
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
