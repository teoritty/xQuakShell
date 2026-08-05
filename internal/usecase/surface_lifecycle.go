package usecase

import (
	domainplugin "xquakshell/internal/domain/plugin"
)

// Reasons a surface closed for something other than the plugin asking. They reach the plugin on
// surface.closed, which is the only way it learns that a stream it is still producing has nowhere
// left to go.
const (
	SurfaceCloseReasonUser    = "closed by user"
	SurfaceCloseReasonSession = "parent session closed"
)

// CloseSurfaceFromUI closes a surface because the user closed its tab. The plugin is told.
func (s *SurfaceService) CloseSurfaceFromUI(surfaceID string) {
	surface, existed := s.registry.Remove(surfaceID)
	if !existed {
		return
	}
	s.announceClosed(surface, SurfaceCloseReasonUser, true)
}

// CloseSurfacesForSession closes every surface bound to a session.
//
// Called synchronously from the session close sequence, before the SSH client goes away — the same
// step that closes channels (ADR-011 §Session lifecycle coupling). The direction is one-way: the
// session owns the surface's lifetime, never the reverse, so closing a surface never touches the
// session or its siblings.
func (s *SurfaceService) CloseSurfacesForSession(sessionID string) {
	for _, surface := range s.registry.RemoveBySession(sessionID) {
		s.announceClosed(surface, SurfaceCloseReasonSession, true)
	}
}

// CloseSurfacesForPlugin closes every surface a plugin process owned.
//
// Called when that process exits or crashes, independently of whether the parent session survives:
// a tab whose producer is gone shows a stream nobody is writing to. The plugin is NOT notified —
// there is nothing left to notify.
func (s *SurfaceService) CloseSurfacesForPlugin(pluginID string) {
	for _, surface := range s.registry.RemoveByPlugin(pluginID) {
		s.announceClosed(surface, "", false)
	}
}

// closeOne closes a surface the plugin itself asked to close. Idempotent: a second close finds
// nothing in the registry and announces nothing, which is what keeps close safe to send from both
// sides without a handshake.
func (s *SurfaceService) closeOne(surfaceID, pluginID, reason string, notifyPlugin bool) {
	surface, existed := s.registry.Remove(surfaceID)
	if !existed || surface.PluginID != pluginID {
		return
	}
	s.announceClosed(surface, reason, notifyPlugin)
}

// announceClosed tells the UI the tab is gone and, when the close did not originate with the
// plugin, tells the plugin too. Both calls happen outside the registry's lock, which is the
// invariant surface_registry.go exists to keep readable.
//
// It is also the single place a surface's output pump is stopped. Every close path — the user, the
// session, the plugin, the plugin's process dying — passes through here, so the goroutine cannot
// outlive its surface down one branch and not another.
func (s *SurfaceService) announceClosed(surface domainplugin.Surface, reason string, notifyPlugin bool) {
	s.output.Close(surface.ID)
	s.presenter.Closed(surface.ID)
	if notifyPlugin && s.outbound != nil {
		s.outbound.Closed(surface.PluginID, surface.ID, reason)
	}
}

// DeliverInput forwards user keystrokes to the surface's owner. Non-interactive kinds are dropped
// silently: a log viewer has no input, and a UI that somehow sent some is not worth an error path
// through the plugin.
func (s *SurfaceService) DeliverInput(surfaceID string, data []byte) {
	surface, ok := s.registry.Lookup(surfaceID)
	if !ok || !surface.Kind.Interactive() || s.outbound == nil {
		return
	}
	s.outbound.Input(surface.PluginID, surface.ID, data)
}

// DeliverResize forwards a terminal geometry change to the surface's owner.
func (s *SurfaceService) DeliverResize(surfaceID string, cols, rows uint16) {
	surface, ok := s.registry.Lookup(surfaceID)
	if !ok || !surface.Kind.Interactive() || s.outbound == nil {
		return
	}
	s.outbound.Resize(surface.PluginID, surface.ID, cols, rows)
}
