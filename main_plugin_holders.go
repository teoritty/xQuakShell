package main

import (
	"context"
	"encoding/json"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// Late-bound ports for the plugin runtime.
//
// Composition here is genuinely circular: the session RPC factory is part of HostConfig, HostConfig
// builds the ProcessHost, the ProcessHost is what PluginManager drives, and every service below
// needs that manager in order to call or notify plugins. Something has to be filled in after the
// fact, and a holder is the smallest something — the port is resolved at CALL time, and a plugin
// process cannot call anything before it has been started by the very manager that fills these in.
//
// They live in their own file because they are wiring, not composition: main_plugins.go says what
// the runtime is made of, and this says how two halves of a cycle reach each other.

// discoveryInboundHolder makes the discovery service reachable from the session RPC factory despite
// being constructed after it, the same push-me-later shape sessionRegistryHolder uses.
//
// The ordering is forced: SessionRPC is part of HostConfig, HostConfig builds the ProcessHost, the
// ProcessHost is what PluginManager drives, and the discovery service needs that manager to notify
// and call plugins. Something has to be late-bound, and this is the smallest something.
//
// A miss returns ErrCapabilityDenied rather than nil: the only way to reach it is a plugin process
// publishing before composition finished, and a denial the plugin can act on beats a nil
// dereference. It is exactly how PluginSessionRPCHandler treats every other unwired inbound port.
type discoveryInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DiscoveryInboundPort
}

func newDiscoveryInboundHolder() *discoveryInboundHolder { return &discoveryInboundHolder{} }

func (h *discoveryInboundHolder) set(port domainplugin.DiscoveryInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *discoveryInboundHolder) Publish(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Publish(ctx, pluginID, params)
}

var _ domainplugin.DiscoveryInboundPort = (*discoveryInboundHolder)(nil)

// surfaceInboundHolder late-binds the surface service into the session RPC factory, for the same
// forced ordering discoveryInboundHolder exists for: the service needs the PluginManager in order
// to notify plugins, and that manager is built from the host this factory is part of.
//
// A miss returns ErrCapabilityDenied rather than nil — the only way to reach it is a plugin
// calling surface.* before composition finished, and a denial it can act on beats a nil
// dereference.
type surfaceInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.SurfaceInboundPort
}

func newSurfaceInboundHolder() *surfaceInboundHolder { return &surfaceInboundHolder{} }

func (h *surfaceInboundHolder) set(port domainplugin.SurfaceInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *surfaceInboundHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Handle(ctx, pluginID, method, params)
}

var _ domainplugin.SurfaceInboundPort = (*surfaceInboundHolder)(nil)

// dialogInboundHolder late-binds the dialog service, for the same forced ordering as the two
// holders above.
type dialogInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DialogInboundPort
}

func newDialogInboundHolder() *dialogInboundHolder { return &dialogInboundHolder{} }

func (h *dialogInboundHolder) set(port domainplugin.DialogInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *dialogInboundHolder) Handle(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.Handle(ctx, pluginID, method, params)
}

var _ domainplugin.DialogInboundPort = (*dialogInboundHolder)(nil)

// detailsInboundHolder late-binds the node-details service, the last of the same family.
type detailsInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.DiscoveryDetailsInboundPort
}

func newDetailsInboundHolder() *detailsInboundHolder { return &detailsInboundHolder{} }

func (h *detailsInboundHolder) set(port domainplugin.DiscoveryDetailsInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *detailsInboundHolder) PublishDetails(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	return port.PublishDetails(ctx, pluginID, params)
}

var _ domainplugin.DiscoveryDetailsInboundPort = (*detailsInboundHolder)(nil)

// discoveryEmitHolder late-binds the frontend emit callback the same way discoveryInboundHolder
// late-binds the inbound port, and for the same forced ordering: the emit coalescer is built inside
// the plugin runtime, and the AppAPI that owns the Wails context is built afterwards from it.
//
// A miss is a no-op, not an error. Between composition and wireEmbed there is no window in which a
// tree can change — no plugin has been told to observe anything yet — and a redraw nobody can
// receive is nothing to report.
type discoveryEmitHolder struct {
	mu   sync.Mutex
	emit func(connectionID, nodeID string)
}

func newDiscoveryEmitHolder() *discoveryEmitHolder { return &discoveryEmitHolder{} }

func (h *discoveryEmitHolder) set(emit func(connectionID, nodeID string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emit = emit
}

// notify runs the callback outside the lock: it reaches into the Wails runtime, and holding a mutex
// across that would serialize every tree update behind an unknown amount of presentation work.
func (h *discoveryEmitHolder) notify(connectionID, nodeID string) {
	h.mu.Lock()
	emit := h.emit
	h.mu.Unlock()
	if emit != nil {
		emit(connectionID, nodeID)
	}
}

// peerTrustInboundHolder late-binds the peer trust service into the session RPC factory, for the
// same forced ordering the holders above exist for: the service needs the session manager, and the
// session manager is built inside the AppAPI that is created from this runtime.
//
// A miss returns ErrCapabilityDenied, never (false, nil). The difference matters here more than
// anywhere else in this file: a nil answer would read as "this peer is not trusted", which is the
// same answer an unknown host gets, and a plugin calling before composition finished would put a
// trust dialog in front of the user with nothing behind it. Denied says what actually happened.
type peerTrustInboundHolder struct {
	mu   sync.Mutex
	port domainplugin.PeerTrustInboundPort
}

func newPeerTrustInboundHolder() *peerTrustInboundHolder { return &peerTrustInboundHolder{} }

func (h *peerTrustInboundHolder) set(port domainplugin.PeerTrustInboundPort) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.port = port
}

func (h *peerTrustInboundHolder) VerifyPeer(
	ctx context.Context,
	pluginID, sessionID, subject string,
	material []byte,
) (bool, error) {
	h.mu.Lock()
	port := h.port
	h.mu.Unlock()
	if port == nil {
		return false, domainplugin.ErrCapabilityDenied
	}
	return port.VerifyPeer(ctx, pluginID, sessionID, subject, material)
}

var _ domainplugin.PeerTrustInboundPort = (*peerTrustInboundHolder)(nil)

// lateBoundPorts is every session RPC port that cannot exist yet when the plugin host is built.
//
// One value instead of five locals: they are created together, filled in together once the
// services they stand for exist, and handed to the factory together. Kept apart, each new one
// added three more lines to the composition function for no decision of its own.
type lateBoundPorts struct {
	discovery *discoveryInboundHolder
	surfaces  *surfaceInboundHolder
	dialogs   *dialogInboundHolder
	details   *detailsInboundHolder
	peerTrust *peerTrustInboundHolder
}

func newLateBoundPorts() lateBoundPorts {
	return lateBoundPorts{
		discovery: newDiscoveryInboundHolder(),
		surfaces:  newSurfaceInboundHolder(),
		dialogs:   newDialogInboundHolder(),
		details:   newDetailsInboundHolder(),
		peerTrust: newPeerTrustInboundHolder(),
	}
}

// sessionRPC names what a plugin process may address. The two ports that exist by now are
// arguments; everything else is a holder resolved at call time.
func (l lateBoundPorts) sessionRPC(
	sessions domainplugin.SessionInboundPort,
	embed *usecase.PluginEmbedInbound,
) sessionRPCPorts {
	return sessionRPCPorts{
		Sessions:  sessions,
		Embed:     embed,
		Discovery: l.discovery,
		Surfaces:  l.surfaces,
		Dialogs:   l.dialogs,
		Details:   l.details,
		PeerTrust: l.peerTrust,
	}
}
