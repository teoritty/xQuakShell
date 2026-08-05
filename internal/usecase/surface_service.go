package usecase

import (
	domainplugin "xquakshell/internal/domain/plugin"
)

// SurfacePresenter delivers a surface's state and bytes to the UI. Implemented in
// internal/presentation/wails; declared here because the use case is the side that knows when
// something happened, and the presentation layer is the side that knows how to say it.
//
// Output returns an error when the UI consumer is not keeping up. That is the one presenter method
// whose failure means something to the plugin — it becomes ErrRateLimited, the same backpressure
// answer session.writeTerminal already gives — while a failure to announce a state change is not
// something a plugin could act on and is therefore not reported.
type SurfacePresenter interface {
	Opened(s domainplugin.Surface)
	Output(surfaceID, dataBase64, stream string) error
	Changed(s domainplugin.Surface)
	Closed(surfaceID string)
}

// SurfaceSessionConnections resolves the connection behind a session. Satisfied by
// *SessionRegistry.
//
// A surface stores the connection, never the session, for anything the frontend will see: the UI
// addresses connections and the host addresses sessions, the separation ADR-014 established for
// discovery and which this reuses rather than reopening.
type SurfaceSessionConnections interface {
	ConnectionForSession(sessionID string) (string, bool)
}

// SurfaceCapabilityLookup returns a plugin's declared ui capability, or nil when it declared none.
type SurfaceCapabilityLookup func(pluginID string) *domainplugin.UICaps

// SurfaceService is the use case behind the surface.* verbs (ADR-015 §1).
//
// It holds no lock of its own: all mutable state lives in SurfaceRegistry, and every method here
// follows the same order — take what the registry knows, release, then call out to the presenter,
// the plugin, or the audit log. That ordering is the reason a slow frontend cannot wedge the
// registry, and it is why the registry and its callers are separate files.
type SurfaceService struct {
	registry  *SurfaceRegistry
	presenter SurfacePresenter
	outbound  domainplugin.SurfaceOutboundPort
	sessions  SurfaceSessionConnections
	caps      SurfaceCapabilityLookup
	audit     domainplugin.SurfaceAuditRecorder
}

// NewSurfaceService wires the surface use case. Every dependency is required except audit, which
// may be nil only where no audit log exists at all — production wiring always supplies one,
// because the open entry is the only lasting record that a tab was a plugin's doing.
func NewSurfaceService(
	registry *SurfaceRegistry,
	presenter SurfacePresenter,
	outbound domainplugin.SurfaceOutboundPort,
	sessions SurfaceSessionConnections,
	caps SurfaceCapabilityLookup,
	audit domainplugin.SurfaceAuditRecorder,
) *SurfaceService {
	if presenter == nil {
		presenter = noopSurfacePresenter{}
	}
	return &SurfaceService{
		registry:  registry,
		presenter: presenter,
		outbound:  outbound,
		sessions:  sessions,
		caps:      caps,
		audit:     audit,
	}
}

// SetPresenter late-binds the UI side, the way SessionLifecycleService.SetChannelBus late-binds
// its own. The composition root builds the plugin stack before the Wails API exists, and a
// service that refused to be constructed until its presenter did would force the two into an
// order the rest of the wiring does not have. Until it is set, a surface's output goes nowhere —
// which is the correct behaviour for a UI that is not up yet, and never a nil dereference.
func (s *SurfaceService) SetPresenter(presenter SurfacePresenter) {
	if presenter == nil {
		return
	}
	s.presenter = presenter
}

// HoldsSurfaces reports whether a plugin currently owns any surface.
//
// It answers the idle sweeper's "is anyone using this?" question, which without it says no for a
// plugin whose only job right now is streaming a log into a tab the user is watching — quiet on
// the RPC channel is exactly what that looks like. The same trap ADR-014 documented for discovery
// bindings, in the same place.
func (s *SurfaceService) HoldsSurfaces(pluginID string) bool {
	return s.registry.CountForPlugin(pluginID) > 0
}

// noopSurfacePresenter drops everything. Used before SetPresenter and in tests that care only
// about the plugin-facing half.
type noopSurfacePresenter struct{}

func (noopSurfacePresenter) Opened(domainplugin.Surface)  {}
func (noopSurfacePresenter) Output(_, _, _ string) error  { return nil }
func (noopSurfacePresenter) Changed(domainplugin.Surface) {}
func (noopSurfacePresenter) Closed(string)                {}

// capsFor returns the plugin's ui capability, or nil.
func (s *SurfaceService) capsFor(pluginID string) *domainplugin.UICaps {
	if s.caps == nil {
		return nil
	}
	return s.caps(pluginID)
}

var _ domainplugin.SurfaceInboundPort = (*SurfaceService)(nil)
var _ domainplugin.SurfaceSessionCloser = (*SurfaceService)(nil)
var _ domainplugin.SurfacePluginCloser = (*SurfaceService)(nil)
