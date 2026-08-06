package wails

import (
	"context"
	"errors"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/usecase"
)

// errDiscoveryUnavailable is returned by the two commanding handlers when no discovery service is
// wired. Reading has no such error on purpose — see GetDiscoveryTree.
var errDiscoveryUnavailable = errors.New("discovery service unavailable")

// DiscoveryTreeService is the slice of the discovery use case this layer needs, named here rather
// than taken as *usecase.DiscoveryService.
//
// It is three methods, and every one of them takes connectionId. That is the boundary written down:
// a handler cannot reach a session-addressed entry point it was never given, and a test can supply
// a recorder instead of assembling a store, an observer, a router, an invoker and a leader in order
// to check which pluginId a click was routed to.
type DiscoveryTreeService interface {
	Snapshot(connectionID string) usecase.DiscoverySnapshot
	SetObserved(connectionID string, nodeIDs []string)
	InvokeAction(ctx context.Context, connectionID, pluginID string, nodeIDs []string, actionID string) error
}

// SetDiscoveryService wires the discovery use case. Passing nil leaves discovery inert rather than
// half-wired.
func (a *AppAPI) SetDiscoveryService(svc DiscoveryTreeService) {
	a.discovery = svc
}

// GetDiscoveryTree returns the connection's plugin-drawn subtrees.
//
// A connection nobody has drawn under — no discovery plugins installed, none started, or the vault
// still locked — yields an empty snapshot and no error, exactly like ListPlugins yields an empty
// list. Emptiness is the normal state of this tree, and reporting it as a failure would put an error
// toast in front of every user who has no discovery plugins at all.
func (a *AppAPI) GetDiscoveryTree(connectionId string) (DiscoverySnapshotDTO, error) {
	if a == nil || a.discovery == nil {
		return discoverySnapshotToDTO(usecase.DiscoverySnapshot{ConnectionID: connectionId}), nil
	}
	return discoverySnapshotToDTO(a.discovery.Snapshot(connectionId)), nil
}

// SetDiscoveryObserved replaces the connection's set of expanded nodes; "" is the connection root.
//
// nodeIds is the FULL current set, never a delta (ADR-014 §data flow). The frontend sends what is
// expanded now, and the host resends it to plugins on every change and on every plugin restart; a
// delta would make one lost message leave a branch empty forever with nothing able to repair it.
func (a *AppAPI) SetDiscoveryObserved(connectionId string, nodeIds []string) error {
	if a == nil || a.discovery == nil {
		return errDiscoveryUnavailable
	}
	if nodeIds == nil {
		// An explicit "nothing is expanded" must survive the JS bridge as an empty set, not as nil
		// that a downstream nil-check could read as "no change".
		nodeIds = []string{}
	}
	a.discovery.SetObserved(connectionId, nodeIds)
	return nil
}

// InvokeDiscoveryAction runs a plugin action on one or more nodes of that plugin's subtree.
//
// pluginId is a required argument, not something the backend infers from the node IDs. Node IDs are
// plugin-chosen and each plugin owns its own tree, so two plugins may legitimately publish a node
// with the same ID under one connection; resolving the owner by searching the trees would pick one
// by map iteration order, and a destructive action would then reach the wrong plugin a fraction of
// the time. The frontend always knows whose subtree was clicked — the snapshot is grouped by
// pluginId — so it states the addressee.
//
// nodeIds is always a list, even for one node: a single action is a mass action over a list of one
// (ADR-014 alternative 3).
func (a *AppAPI) InvokeDiscoveryAction(connectionId string, pluginId string, nodeIds []string, actionId string) error {
	if a == nil || a.discovery == nil {
		return errDiscoveryUnavailable
	}
	// No timeout is imposed here: the invoker bounds the plugin call with the ADR's 5 s ack timeout,
	// and a second, shorter deadline layered on top would only make this layer the one that decides
	// how long an acknowledgement may take.
	return a.discovery.InvokeAction(a.reqCtx(), connectionId, pluginId, nodeIds, actionId)
}

// EmitDiscoveryTreeChanged tells the frontend that one branch of a connection's tree changed.
//
// It carries the node as well as the connection because the coalescing window upstream is per node:
// a connection whose branches are all filling at once produces one event per branch, and a payload
// without the node would leave the frontend able only to refetch the whole tree each time — turning
// a limit on re-rendering into a limit on event count.
//
// The event carries no snapshot. The frontend calls GetDiscoveryTree for the connection, so one
// reader path serves both the first paint and every update, and an event that arrives while the
// tree is being torn down cannot deliver a tree that no longer exists.
func (a *AppAPI) EmitDiscoveryTreeChanged(connectionID, nodeID string) {
	if a == nil || a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventDiscoveryTreeChanged, DiscoveryTreeChangedPayload{
		ConnectionID: connectionID,
		NodeID:       nodeID,
	})
}
