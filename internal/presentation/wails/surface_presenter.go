package wails

import (
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	domainplugin "xquakshell/internal/domain/plugin"
)

// SurfacePresenter turns surface state changes into frontend events.
//
// It is a type of its own rather than four more methods on AppAPI, and the reason is the binding
// surface: every exported AppAPI method becomes a Wails binding (app_bindings_test.go enforces
// that each one has a delegate), and "Opened", "Output", "Changed", "Closed" are exactly the kind
// of generic names that should not appear in a frontend's API. It holds the AppAPI only for the
// Wails context.
type SurfacePresenter struct {
	api *AppAPI
}

// NewSurfacePresenter creates the presenter for the surface use case.
func NewSurfacePresenter(api *AppAPI) *SurfacePresenter {
	return &SurfacePresenter{api: api}
}

// Opened announces a new tab.
func (p *SurfacePresenter) Opened(s domainplugin.Surface) {
	p.emit(EventPluginSurfaceOpened, surfaceToDTO(s))
}

// Changed announces a title or state change on an existing tab.
func (p *SurfacePresenter) Changed(s domainplugin.Surface) {
	p.emit(EventPluginSurfaceChanged, surfaceToDTO(s))
}

// Closed announces that a tab is gone. No reason travels with it: the tab disappears, and telling
// a user that the tab they just closed is closed would be noise.
func (p *SurfacePresenter) Closed(surfaceID string) {
	p.emit(EventPluginSurfaceClosed, SurfaceClosedPayload{SurfaceID: surfaceID})
}

// Output delivers one batch of a surface's stream.
//
// It reports nothing, because there is nothing here to report to: this is called from the
// surface's output pump, long after the plugin's write returned. Wails events are fire and forget
// — EventsEmit has no completion to read a verdict from — so whether the plugin may keep writing
// is decided by the bounded queue in front of the pump, which is the only place that can observe
// the consumer falling behind at all.
func (p *SurfacePresenter) Output(surfaceID, dataBase64, stream string) {
	p.emit(EventPluginSurfaceOutput, SurfaceOutputPayload{
		SurfaceID: surfaceID,
		Data:      dataBase64,
		Stream:    stream,
	})
}

func (p *SurfacePresenter) emit(event string, payload any) {
	if p == nil || p.api == nil || p.api.ctx == nil {
		return
	}
	wailsrt.EventsEmit(p.api.ctx, event, payload)
}
