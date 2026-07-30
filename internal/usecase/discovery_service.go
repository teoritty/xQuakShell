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

// ApplyPublish routes one plugin snapshot into the tree.
func (s *DiscoveryService) ApplyPublish(ctx context.Context, pluginID string, payload DiscoveryPublish) error {
	return s.publish.Apply(ctx, pluginID, payload)
}

// Publish implements domainplugin.DiscoveryInboundPort. The empty result is deliberate: publish is
// a notification, and there is nothing meaningful to hand back to a plugin that already knows what
// it sent.
func (s *DiscoveryService) Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	payload, err := DecodeDiscoveryPublish(params)
	if err != nil {
		return nil, err
	}
	return nil, s.ApplyPublish(ctx, pluginID, payload)
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
		s.publish.ForgetPlugin(connectionID, pluginID)
	}
}

// Snapshot returns a connection's tree for rendering.
func (s *DiscoveryService) Snapshot(connectionID string) DiscoverySnapshot {
	return s.store.Snapshot(connectionID)
}

var _ domainplugin.DiscoveryInboundPort = (*DiscoveryService)(nil)
var _ domainplugin.DiscoveryConnectionClearer = (*DiscoveryService)(nil)
