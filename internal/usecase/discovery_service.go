package usecase

import (
	"context"
	"encoding/json"

	domainplugin "xquakshell/internal/domain/plugin"
)

// DiscoveryService is the one entry point the rest of the app uses for discovery subtrees.
//
// It holds no state and makes no decisions: every method here names the collaborator that owns the
// concern and gets out of the way. Everything outside is addressed by connectionID — no sessionID
// crosses this boundary in either direction (ADR-014).
type DiscoveryService struct {
	store    *DiscoveryStore
	observer *DiscoveryObserver
	publish  *DiscoveryPublishRouter
	invoker  *DiscoveryInvoker
}

// NewDiscoveryService assembles the facade over already-constructed parts.
func NewDiscoveryService(store *DiscoveryStore, observer *DiscoveryObserver, publish *DiscoveryPublishRouter, invoker *DiscoveryInvoker) *DiscoveryService {
	return &DiscoveryService{store: store, observer: observer, publish: publish, invoker: invoker}
}

// SetObserved replaces a connection's set of expanded nodes; "" is the connection root.
func (s *DiscoveryService) SetObserved(connectionID string, nodeIDs []string) {
	s.observer.SetObserved(connectionID, nodeIDs)
}

// applyPublish routes one plugin snapshot into the tree.
//
// It is deliberately UNEXPORTED. Publish below is the only way in from outside this package, and it
// is reached exclusively through PluginSessionRPCHandler, which authorizes the payload's sessionId
// against the plugin's bindings first. An exported entry point beside it would be a second door
// into the same room with no lock on it — and the presentation layer wiring up next would have no
// way to tell, from the signature alone, that it was the unauthorized one.
func (s *DiscoveryService) applyPublish(ctx context.Context, pluginID string, payload DiscoveryPublish) error {
	return s.publish.Apply(ctx, pluginID, payload)
}

// Publish implements domainplugin.DiscoveryInboundPort. It is a request rather than a notification
// (ADR-014 §data flow): the host has nothing meaningful to tell a plugin about a snapshot the
// plugin composed, but publish names a sessionId and must therefore be refusable — by the gate with
// -32001, or by the IDOR check upstream. A notification would carry no refusal back, leaving a
// denied plugin unable to tell rejection from a lost message.
//
// The nil result is the caller's cue to answer {"ok":true}: accepted for processing, which is not
// the same as rendered. A snapshot for a collapsed branch or a session that has stopped leading is
// accepted here and then dropped, by design.
func (s *DiscoveryService) Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	payload, err := DecodeDiscoveryPublish(params)
	if err != nil {
		return nil, err
	}
	return nil, s.applyPublish(ctx, pluginID, payload)
}

// InvokeAction relays a (possibly mass) action on nodes of one plugin's subtree. pluginID is
// explicit because node IDs are only unique within a plugin's own tree; see DiscoveryInvoker.
func (s *DiscoveryService) InvokeAction(ctx context.Context, connectionID, pluginID string, nodeIDs []string, actionID string) error {
	return s.invoker.InvokeAction(ctx, connectionID, pluginID, nodeIDs, actionID)
}

// ClearConnection implements domainplugin.DiscoveryConnectionClearer, dropping everything keyed by
// the connection: the tree, the observed set that pointed into it, and its pace windows.
func (s *DiscoveryService) ClearConnection(connectionID string) {
	s.store.ClearConnection(connectionID)
	s.observer.ClearConnection(connectionID)
	s.publish.ForgetConnection(connectionID)
}

// ClearPlugin drops one plugin's subtree from every connection it drew under, leaving the other
// plugins' subtrees on those connections untouched. It is the deactivation path: a plugin the user
// stopped, disabled, or uninstalled must stop occupying the tree immediately, because nothing is
// left that could confirm the resources it listed still exist.
//
// It is deliberately not reachable from a plugin: the argument is the plugin's own ID, supplied by
// the lifecycle that stopped it.
func (s *DiscoveryService) ClearPlugin(pluginID string) {
	for _, connectionID := range s.store.ConnectionsWithPlugin(pluginID) {
		removed := s.store.ClearPlugin(connectionID, pluginID)
		// Same order as the publish path: the observed set sheds the vanished IDs before anything
		// is told the tree changed.
		if len(removed) > 0 {
			s.observer.Retain(connectionID, removed)
		}
		s.publish.EmitConnectionChanged(connectionID)
	}
	// Outside the loop, because a stopped plugin's pace windows outlive its trees: a publish that
	// was counted and then dropped (collapsed branch, non-leading session) leaves a window and no
	// tree, so sweeping only the connections it drew under would strand exactly those.
	s.publish.ForgetPlugin(pluginID)
}

// MarkPluginStale flags every branch a plugin drew as stale, under every connection, touching no
// other plugin's branches and deleting nothing.
//
// It covers both ways a running plugin stops answering without being stopped by the user: a crash,
// and an idle suspension. The two differ in intent and in nothing else that matters here — in both
// the process is gone, the tree on screen is its last answer, and the supervisor or the next
// activation brings it back, at which point the replayed observed set refills the branches. That is
// why this is deliberately NOT ClearPlugin: a teardown would turn a recoverable absence into a
// subtree that visibly vanishes and returns.
//
// Stale is more than a label. It is what refuses an action inside the branch, and refusing is the
// honest answer: the alternative is an invokeAction dispatched into a dead process and failing on
// the 5 s ack timeout, which tells the user nothing about why.
func (s *DiscoveryService) MarkPluginStale(pluginID string) {
	for _, connectionID := range s.store.ConnectionsWithPlugin(pluginID) {
		if s.store.MarkPluginStale(connectionID, pluginID) {
			s.publish.EmitConnectionChanged(connectionID)
		}
	}
}

// Snapshot returns a connection's tree for rendering.
func (s *DiscoveryService) Snapshot(connectionID string) DiscoverySnapshot {
	return s.store.Snapshot(connectionID)
}

var _ domainplugin.DiscoveryInboundPort = (*DiscoveryService)(nil)
var _ domainplugin.DiscoveryConnectionClearer = (*DiscoveryService)(nil)
