package wails

import (
	"errors"
	"strings"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	domainplugin "xquakshell/internal/domain/plugin"
)

var errDialogsUnavailable = errors.New("plugin dialogs unavailable")

// DialogCommands is the slice of the dialog use case this layer needs: the two answers a user can
// give. Named here so a handler cannot reach the plugin-facing half.
type DialogCommands interface {
	Submit(dialogID string, values map[string]string) error
	Cancel(dialogID string) error
}

// SetDialogService wires the dialog use case. Passing nil leaves dialogs inert.
func (a *AppAPI) SetDialogService(svc DialogCommands) {
	if a == nil {
		return
	}
	a.dialogs = svc
}

// SubmitPluginDialog delivers a form's answer.
//
// A validation failure comes back as an error the modal shows in place, which is why this returns
// one rather than swallowing it: the dialog stays open and the user corrects the field.
func (a *AppAPI) SubmitPluginDialog(dialogId string, values map[string]string) error {
	if a == nil || a.dialogs == nil {
		return errDialogsUnavailable
	}
	id := strings.TrimSpace(dialogId)
	if id == "" {
		return errInvalidSurfaceRequest
	}
	if values == nil {
		values = map[string]string{}
	}
	return a.dialogs.Submit(id, values)
}

// CancelPluginDialog closes a dialog without an answer.
func (a *AppAPI) CancelPluginDialog(dialogId string) error {
	if a == nil || a.dialogs == nil {
		return errDialogsUnavailable
	}
	id := strings.TrimSpace(dialogId)
	if id == "" {
		return errInvalidSurfaceRequest
	}
	return a.dialogs.Cancel(id)
}

// DialogPresenter shows a plugin's modal. Its own type for the same reason SurfacePresenter is:
// every exported AppAPI method becomes a Wails binding, and these are not frontend API.
type DialogPresenter struct {
	api *AppAPI
}

// NewDialogPresenter creates the presenter for the dialog use case.
func NewDialogPresenter(api *AppAPI) *DialogPresenter { return &DialogPresenter{api: api} }

// DialogOpened shows the modal.
func (p *DialogPresenter) DialogOpened(d domainplugin.Dialog) {
	p.emit(EventPluginDialogOpened, dialogToDTO(d))
}

// DialogClosed ends the interaction, unlike DialogError, which leaves the
// dialog up so the user can correct their input. Anything typed is gone by the
// time this arrives.
func (p *DialogPresenter) DialogClosed(dialogID string) {
	p.emit(EventPluginDialogClosed, DialogClosedPayload{DialogID: dialogID})
}

// DialogError shows a failure the plugin reported — "docker refused this name" — without closing
// the dialog, so the user can correct what they entered.
func (p *DialogPresenter) DialogError(dialogID, message string, fieldErrors map[string]string) {
	if fieldErrors == nil {
		fieldErrors = map[string]string{}
	}
	p.emit(EventPluginDialogError, DialogErrorPayload{
		DialogID:    dialogID,
		Message:     message,
		FieldErrors: fieldErrors,
	})
}

func (p *DialogPresenter) emit(event string, payload any) {
	if p == nil || p.api == nil || p.api.ctx == nil {
		return
	}
	wailsrt.EventsEmit(p.api.ctx, event, payload)
}
