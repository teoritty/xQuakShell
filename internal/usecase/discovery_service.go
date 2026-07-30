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

// InvokeAction relays a (possibly mass) action on nodes of a connection's tree.
func (s *DiscoveryService) InvokeAction(ctx context.Context, connectionID string, nodeIDs []string, actionID string) error {
	return s.invoker.InvokeAction(ctx, connectionID, nodeIDs, actionID)
}

// ClearConnection implements domainplugin.DiscoveryConnectionClearer, dropping both the tree and
// the observed set that pointed into it.
func (s *DiscoveryService) ClearConnection(connectionID string) {
	s.store.ClearConnection(connectionID)
	s.observer.ClearConnection(connectionID)
}

// Snapshot returns a connection's tree for rendering.
func (s *DiscoveryService) Snapshot(connectionID string) DiscoverySnapshot {
	return s.store.Snapshot(connectionID)
}

var _ domainplugin.DiscoveryInboundPort = (*DiscoveryService)(nil)
var _ domainplugin.DiscoveryConnectionClearer = (*DiscoveryService)(nil)
