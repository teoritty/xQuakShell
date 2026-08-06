package main

import (
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// The ADR-015 half of the plugin runtime: plugin-owned tabs, modal dialogs and the property panel
// of a discovery node.
//
// It is assembled here rather than inline in newPluginRuntime because it is one subject with one
// reason to change. What it needs from the rest of the runtime is listed explicitly below, which
// is also the honest statement of how much of the runtime this feature actually touches.

// uiStack is the three services that make up capabilities.ui.
type uiStack struct {
	surfaces    *usecase.SurfaceService
	dialogs     *usecase.DialogService
	nodeDetails *usecase.DiscoveryDetailsService
}

// uiStackDeps is everything the ui services borrow from the rest of the runtime.
type uiStackDeps struct {
	// Manager reaches the plugin process: every host->plugin notification goes through it.
	Manager *usecase.PluginManager
	// Registry answers what a plugin declared, which is what every ui grant is checked against.
	Registry *usecase.PluginRegistry
	// Sessions resolves the connection behind a session, so a surface can be displayed under one
	// without the frontend ever seeing a session id (ADR-014's separation, reused).
	Sessions usecase.SurfaceSessionConnections
	Audit    *usecase.PluginAuditWriter
	// Store, Leader and Pace belong to discovery: node details is a discovery verb that happens to
	// draw a panel, so it shares the tree, the leading session and the publish budget rather than
	// keeping its own idea of any of them.
	Store  *usecase.DiscoveryStore
	Leader *usecase.DiscoveryLeader
	Pace   *usecase.DiscoveryPace
}

// buildUIStack constructs the three services and registers them with their inbound holders.
//
// Presenters are left nil: the AppAPI does not exist yet, and wireEmbed pushes them in when it
// does. Until then a surface's output goes into its queue and a dialog opens on no screen, which
// is the correct behaviour for a UI that is not up rather than a nil dereference.
func buildUIStack(
	deps uiStackDeps,
	surfaceInbound *surfaceInboundHolder,
	dialogInbound *dialogInboundHolder,
	detailsInbound *detailsInboundHolder,
) uiStack {
	surfaces := usecase.NewSurfaceService(
		usecase.NewSurfaceRegistry(),
		nil,
		usecase.NewSurfaceNotifier(deps.Manager),
		deps.Sessions,
		deps.Registry.UICapabilities,
		deps.Audit.SurfaceFunc(),
	)
	surfaceInbound.set(surfaces)

	// Dialogs share the surface capability lookup: both ask the same manifest the same question.
	dialogs := usecase.NewDialogService(
		usecase.NewDialogRegistry(),
		nil,
		usecase.NewDialogNotifier(deps.Manager),
		deps.Registry.UICapabilities,
		deps.Audit.DialogFunc(),
	)
	dialogInbound.set(dialogs)

	// Node details (ADR-015 §3): a discovery verb addressing a node and a ui verb drawing a panel,
	// which is why it needs both grants and reuses both features' dependencies — including the
	// publish budget, so a plugin cannot spend past its discovery limit through this door.
	details := usecase.NewDiscoveryDetailsService(
		deps.Store,
		deps.Leader,
		deps.Manager,
		deps.Audit.DiscoveryFunc(),
		deps.Registry.UICapabilities,
		deps.Pace,
	)
	detailsInbound.set(details)

	return uiStack{surfaces: surfaces, dialogs: dialogs, nodeDetails: details}
}

// wirePluginProcessLifecycle decides what happens to a plugin's drawings when its process starts,
// stops, crashes or is suspended.
//
// Discovery and ui answer differently on purpose, and the two answers are next to each other here
// so the difference is visible rather than inferred from two distant call sites:
//
// A discovery branch is a snapshot. A restarted plugin refills it from the replayed observed set,
// so a crash or a suspension marks it stale rather than deleting it — marking also refuses actions
// inside it, which beats dispatching into a dead process and waiting out the ack.
//
// A surface is a stream, and there is no replay. A process that is gone takes its tabs with it on
// every transition, because a tab showing a stream nobody is producing is worse than no tab. A
// dialog goes the same way: a modal whose owner cannot answer must not stay on screen.
func wirePluginProcessLifecycle(
	manager *usecase.PluginManager,
	supervisor *usecase.PluginSupervisor,
	observer *usecase.DiscoveryObserver,
	discovery *usecase.DiscoveryService,
	leader *usecase.DiscoveryLeader,
	ui uiStack,
) {
	// A restarted plugin is told the whole observed set again; without this the level-triggered
	// contract silently degrades into an edge-triggered one (ADR-014 §data flow).
	manager.SetProcessStartedHandler(observer.PluginStarted)

	// A plugin the user disabled or uninstalled loses its subtree at once, under every connection,
	// without touching its neighbours' (ADR-014).
	manager.SetProcessStoppedHandler(func(pluginID string) {
		discovery.ClearPlugin(pluginID)
		ui.surfaces.CloseSurfacesForPlugin(pluginID)
		ui.dialogs.CancelForPlugin(pluginID)
	})

	crashOrSuspend := func(pluginID string) {
		discovery.MarkPluginStale(pluginID)
		ui.surfaces.CloseSurfacesForPlugin(pluginID)
		ui.dialogs.CancelForPlugin(pluginID)
	}
	manager.SetProcessCrashedHandler(crashOrSuspend)
	manager.SetProcessSuspendedHandler(crashOrSuspend)

	// Once the supervisor stops trying, stale stops being true. The branches become error with a
	// reason, so "restarting" and "given up" are not the same grey subtree (ADR-014 §Leading
	// session).
	supervisor.SetGaveUpHandler(discovery.MarkPluginUnrecoverable)

	// And what makes that story reachable at all: a plugin drawing something is in use, even though
	// it holds no session and owns no view panel. Without this the idle sweeper reclaimed it after
	// five quiet minutes and the supervisor declined to restart it, so its branches went stale and
	// stayed there. An open surface counts for the same reason and against the same trap: a plugin
	// streaming a log into a tab the user is watching is silent on the RPC channel, which is
	// exactly what idle looks like from here.
	manager.SetPluginRetentionChecker(func(pluginID string) bool {
		return leader.HoldsBindings(pluginID) || ui.surfaces.HoldsSurfaces(pluginID)
	})
}

// wireUIPresenters connects the ui services to the frontend, once there is one.
//
// Both directions are set here: the AppAPI is the presenter the use cases push tabs, bytes and
// modals to, and the use cases are what the AppAPI's own handlers call when the user types into a
// surface, closes one, or answers a dialog. The session close cascade goes through the session
// manager, beside the channel bus it runs next to.
func (r *pluginRuntime) wireUIPresenters(api *presentation.AppAPI) {
	if r.surfaces != nil {
		r.surfaces.SetPresenter(presentation.NewSurfacePresenter(api))
		api.SetSurfaceService(r.surfaces)
		api.Sessions().SetSurfaces(r.surfaces)
	}
	if r.dialogs != nil {
		r.dialogs.SetPresenter(presentation.NewDialogPresenter(api))
		api.SetDialogService(r.dialogs)
	}
	if r.nodeDetails != nil {
		r.nodeDetails.SetEmitter(api.EmitNodeDetailsChanged)
		api.SetNodeDetailsService(r.nodeDetails)
	}
}
