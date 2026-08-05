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
	return &SurfaceService{
		registry:  registry,
		presenter: presenter,
		outbound:  outbound,
		sessions:  sessions,
		caps:      caps,
		audit:     audit,
	}
}

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
