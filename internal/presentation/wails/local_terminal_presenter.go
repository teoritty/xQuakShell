package wails

import (
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// LocalTerminalOutputPayload carries one batch of a shell's output.
type LocalTerminalOutputPayload struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

// LocalTerminalClosedPayload names the tab that is gone.
type LocalTerminalClosedPayload struct {
	ID string `json:"id"`
}

// LocalTerminalPresenter turns a shell's output and its ending into frontend events.
//
// A type of its own for the same reason SurfacePresenter is one: every exported AppAPI method
// becomes a Wails binding, and "Output" and "Closed" are exactly the generic names that must not
// appear on a frontend's API surface.
type LocalTerminalPresenter struct {
	api *AppAPI
}

// NewLocalTerminalPresenter creates the presenter for the local terminal use case.
func NewLocalTerminalPresenter(api *AppAPI) *LocalTerminalPresenter {
	return &LocalTerminalPresenter{api: api}
}

// Output delivers one batch of a shell's bytes.
//
// Nothing is reported back. A Wails event is fire and forget, and unlike the plugin surface path
// there is nothing here that would act on a verdict anyway: the producer is a read loop behind a
// bounded channel, so a consumer falling behind is already answered by that channel filling up.
func (p *LocalTerminalPresenter) Output(id, dataBase64 string) {
	p.emit(EventLocalTerminalOutput, LocalTerminalOutputPayload{ID: id, Data: dataBase64})
}

// Closed announces that a shell ended, whichever side ended it.
func (p *LocalTerminalPresenter) Closed(id string) {
	p.emit(EventLocalTerminalClosed, LocalTerminalClosedPayload{ID: id})
}

func (p *LocalTerminalPresenter) emit(event string, payload any) {
	if p == nil || p.api == nil || p.api.ctx == nil {
		return
	}
	wailsrt.EventsEmit(p.api.ctx, event, payload)
}
