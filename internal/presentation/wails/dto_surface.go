package wails

import domainplugin "xquakshell/internal/domain/plugin"

// SurfaceDTO is one plugin-owned tab as the frontend sees it (ADR-015 §1).
//
// There is no sessionId here, and there must not be: the UI addresses a surface by SurfaceID and
// a connection by ConnectionID, while the session is how the HOST reaches the plugin. That is the
// same separation ADR-014 established for discovery, and putting a session id on this payload
// would reopen it for the sake of a field nothing in the frontend can use.
type SurfaceDTO struct {
	SurfaceID    string `json:"surfaceId"`
	ConnectionID string `json:"connectionId"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	IconID       string `json:"iconId"`
	PluginID     string `json:"pluginId"`
	State        string `json:"state"`
	ErrorMessage string `json:"errorMessage"`
}

func surfaceToDTO(s domainplugin.Surface) SurfaceDTO {
	return SurfaceDTO{
		SurfaceID:    s.ID,
		ConnectionID: s.ConnectionID,
		Kind:         string(s.Kind),
		Title:        s.Title,
		IconID:       s.IconID,
		PluginID:     s.PluginID,
		State:        s.State,
		ErrorMessage: s.Error,
	}
}

// SurfaceOutputPayload carries one chunk of a surface's stream. The bytes stay base64 all the way
// to the renderer, exactly as TerminalOutputPayload does: decoding early would mean choosing an
// encoding for bytes that have none.
type SurfaceOutputPayload struct {
	SurfaceID string `json:"surfaceId"`
	Data      string `json:"data"`
	// Stream is "stdout" or "stderr". It is a field rather than something the viewer infers,
	// because by the time bytes reach a renderer the distinction is unrecoverable.
	Stream string `json:"stream"`
}

// SurfaceClosedPayload names the surface that is gone. No reason is carried: the tab disappears,
// and a toast explaining that a tab the user just closed is closed would be noise.
type SurfaceClosedPayload struct {
	SurfaceID string `json:"surfaceId"`
}
