package main

import (
	"xquakshell/internal/usecase"
)

// Discovery subtrees (ADR-014): the store, the observer, the leader and the pacing around them.
//
// Assembled here rather than inline in newPluginRuntime because it is one subject with one reason
// to change, and because the ui services (main_plugin_ui.go) borrow half of it — a node details
// panel is a discovery verb that happens to draw a form.

// discoveryStack is what one connection's plugin subtrees are made of.
type discoveryStack struct {
	store    *usecase.DiscoveryStore
	observer *usecase.DiscoveryObserver
	leader   *usecase.DiscoveryLeader
	service  *usecase.DiscoveryService
	pace     *usecase.DiscoveryPace
	// emit is late-bound: tree changes reach the frontend only once wireEmbed has an AppAPI to
	// hand over.
	emit *discoveryEmitHolder
}

// discoveryStackDeps is what discovery borrows from the rest of the runtime.
type discoveryStackDeps struct {
	Manager  *usecase.PluginManager
	Registry *usecase.PluginRegistry
	Sessions *sessionRegistryHolder
	Audit    *usecase.PluginAuditWriter
}

// buildDiscoveryStack constructs the stack and registers it with its inbound holder.
func buildDiscoveryStack(deps discoveryStackDeps, inbound *discoveryInboundHolder) discoveryStack {
	registry, manager := deps.Registry, deps.Manager
	sessionRegistry, pluginAudit := deps.Sessions, deps.Audit

	// Discovery subtrees (ADR-014). The observer and the leader need each other — the observer must
	// resolve a connection to its leading session, the leader must resend the observed set on
	// handover — so the observer is built first and the leader pushed into it, the same late-binding
	// shape the channel bus uses. Tree changes reach the frontend through discoveryEmit, filled in
	// by wireEmbed once the AppAPI exists.
	discoveryStore := usecase.NewDiscoveryStore()
	discoveryObserver := usecase.NewDiscoveryObserver(registry, manager)
	discoveryEmit := newDiscoveryEmitHolder()
	// Pace is built before the leader because both the publish path and connection teardown need
	// it, and teardown lives on the leader.
	discoveryPace := usecase.NewDiscoveryPace(
		usecase.NewDiscoveryPublishLimiter(nil),
		usecase.NewDiscoveryEmitCoalescer(discoveryEmit.notify, nil, nil),
	)
	// The leader owns the plugin lifecycle for discovery, and it is the only thing that can: a
	// discovery plugin draws under a core SSH connection it does not own, so the session bridge —
	// which binds a plugin to the session it provides — never binds it, and every publish would be
	// refused by the IDOR check. registry and manager are constructor arguments so this cannot be
	// left unwired without failing to compile.
	// The last argument is discoveryEmit.notify, not nil. A handover marks every branch stale and
	// the backend starts refusing actions inside them from that moment; with no callback the frontend
	// was never told, so the rows still looked live and a click came back with "branch is stale or
	// failed" and nothing on screen explaining why. It bypasses the coalescer deliberately — see
	// NewDiscoveryLeader.
	discoveryLeader := usecase.NewDiscoveryLeader(sessionRegistry, registry, manager, discoveryStore, discoveryObserver, discoveryPace, discoveryEmit.notify)
	discoveryObserver.SetLeader(discoveryLeader)
	discoveryService := usecase.NewDiscoveryService(
		discoveryStore,
		discoveryObserver,
		// The registry supplies the declared icon IDs: an iconId no manifest registered is dropped
		// from the node rather than failing the publish (ADR-014 §Security model).
		usecase.NewDiscoveryPublishRouter(discoveryStore, discoveryObserver, discoveryLeader, discoveryPace, registry),
		usecase.NewDiscoveryInvoker(discoveryStore, discoveryLeader, manager, pluginAudit.DiscoveryFunc()),
	)
	// Now the plugin->host half can be reached: discovery.publish arrives through the session RPC
	// handler, which authorizes the sessionId before anything here sees the snapshot.
	inbound.set(discoveryService)

	return discoveryStack{
		store:    discoveryStore,
		observer: discoveryObserver,
		leader:   discoveryLeader,
		service:  discoveryService,
		pace:     discoveryPace,
		emit:     discoveryEmit,
	}
}
