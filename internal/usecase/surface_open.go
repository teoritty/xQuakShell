package usecase

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// open handles surface.open (ADR-015 §1).
//
// The checks run cheapest-first and nothing is registered until every one has passed, so a refusal
// leaves no half-open tab and no audit entry claiming a surface that does not exist. The parent
// session's binding was already checked one layer up; what is left here is the plugin's own
// declaration, the kind, and whether the host still knows the session at all.
func (s *SurfaceService) open(pluginID string, req surfaceOpenParams) (json.RawMessage, error) {
	kind, err := domainplugin.ParseSurfaceKind(req.Kind)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domainplugin.ErrCapabilityDenied, err)
	}
	caps := s.capsFor(pluginID)
	if !caps.AllowsSurfaceKind(string(kind)) {
		return nil, fmt.Errorf("%w: surface kind %q not declared", domainplugin.ErrCapabilityDenied, kind)
	}

	sessionID := strings.TrimSpace(req.ParentSessionID)
	connectionID, known := s.sessions.ConnectionForSession(sessionID)
	if !known {
		// The session was authorized a moment ago and is already gone: an ordinary teardown race,
		// reported as a plain failure rather than a denial, because nothing was refused.
		return nil, fmt.Errorf("surface: parent session is no longer available")
	}

	surface := domainplugin.Surface{
		ID:              newSurfaceID(),
		PluginID:        pluginID,
		ParentSessionID: sessionID,
		ConnectionID:    connectionID,
		Kind:            kind,
		Title:           sanitizeSurfaceTitle(req.Title),
		IconID:          strings.TrimSpace(req.IconID),
		State:           domainplugin.SurfaceStateConnecting,
	}

	if err := s.registry.Add(surface, caps.EffectiveMaxSurfaces()); err != nil {
		s.recordOpen(surface, err)
		return nil, err
	}
	s.recordOpen(surface, nil)
	s.presenter.Opened(surface)
	return json.Marshal(map[string]string{"surfaceId": surface.ID})
}

// sanitizeSurfaceTitle strips control characters and bidirectional overrides, then bounds the
// length in runes.
//
// Truncation, not refusal: discovery rejects an over-long Label because a node with no valid label
// has nothing to draw, whereas a tab with a shortened title is exactly as usable as the plugin
// intended. Losing the surface over a cosmetic field would be the worse answer.
func sanitizeSurfaceTitle(title string) string {
	clean := discovery.SanitizeText(strings.TrimSpace(title))
	runes := []rune(clean)
	if len(runes) > domainplugin.MaxSurfaceTitleLen {
		runes = runes[:domainplugin.MaxSurfaceTitleLen]
	}
	return string(runes)
}

// recordOpen writes the one audited surface event. It runs for a refused open too: a plugin that
// tried to claim a tab and was turned away is exactly what an incident review looks for, and an
// entry written only on success would omit it.
func (s *SurfaceService) recordOpen(surface domainplugin.Surface, err error) {
	if s.audit == nil {
		return
	}
	entry := domainplugin.SurfaceAuditEntry{
		Timestamp:       time.Now(),
		PluginID:        surface.PluginID,
		SurfaceID:       surface.ID,
		ParentSessionID: surface.ParentSessionID,
		ConnectionID:    surface.ConnectionID,
		Kind:            string(surface.Kind),
		Success:         err == nil,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	s.audit(entry)
}
