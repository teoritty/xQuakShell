package wails

import (
	"encoding/base64"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// errLocalTerminalsUnavailable is what every handler returns when no service is wired, matching
// how surfaces report the same condition.
var errLocalTerminalsUnavailable = errors.New("local terminals unavailable")

// errInvalidLocalTerminalRequest covers every rejected argument. One generic message on purpose:
// the caller is untrusted code in a WebView, and a specific message tells it which part of a
// probe landed.
var errInvalidLocalTerminalRequest = errors.New("invalid local terminal request")

// LocalTerminalCommands is the slice of the local terminal use case this layer needs.
//
// Note what is not on it: nothing that names a program, a path or an argument. Open takes no
// input at all. That is the security property this feature rests on, and stating it as a type
// means a future handler cannot quietly acquire one.
type LocalTerminalCommands interface {
	Open() (usecase.LocalTerminalInfo, error)
	Write(id string, data []byte) error
	Resize(id string, cols, rows uint16) error
	Close(id string) error
	// CloseAll ends every shell at shutdown. On the interface rather than reached through the
	// facade because app.go may not import usecase (ADR-010), and a shell left running after the
	// window closed has nothing on screen to stop it.
	CloseAll()
}

// SetLocalShellCatalog wires the list of shells this machine has.
//
// Separate from SetLocalTerminalService because listing the shells that COULD be started and
// running the ones that ARE are different subjects. Routing the list through the service made it
// hold a collaborator for one pass-through call, which is a second reason for that file to change.
func (a *AppAPI) SetLocalShellCatalog(catalog domain.ShellCatalog) {
	a.localShells = catalog
}

// SetLocalTerminalService wires the local terminal use case.
//
// A setter rather than a constructor parameter: NewAppAPI already carries thirty of those and is
// recorded in the budget baseline, so another one would have to grow a number that is meant only
// to shrink.
func (a *AppAPI) SetLocalTerminalService(svc LocalTerminalCommands) {
	a.localTerminals = svc
}

// LocalTerminalDTO is a tab the frontend should draw for a running shell.
type LocalTerminalDTO struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ShellOptionDTO is one entry in the settings picker.
type ShellOptionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OpenLocalTerminal starts a shell on this machine and returns the tab for it.
//
// The empty signature is the point and must stay empty. Nothing the frontend sends can influence
// which program runs: the shell comes from settings by id, and that id is resolved against a
// closed set in infra. A parameter here - a path, a command, "just the arguments" - would turn a
// feature with no injection surface into one with the usual one.
func (a *AppAPI) OpenLocalTerminal() (LocalTerminalDTO, error) {
	if a.localTerminals == nil {
		return LocalTerminalDTO{}, errLocalTerminalsUnavailable
	}
	info, err := a.localTerminals.Open()
	if err != nil {
		return LocalTerminalDTO{}, err
	}
	return LocalTerminalDTO{ID: info.ID, Title: info.Title}, nil
}

// CloseLocalTerminal ends a shell because the user closed its tab.
func (a *AppAPI) CloseLocalTerminal(id string) error {
	if a.localTerminals == nil {
		return errLocalTerminalsUnavailable
	}
	if id == "" {
		return errInvalidLocalTerminalRequest
	}
	return a.localTerminals.Close(id)
}

// SendLocalTerminalInput forwards keystrokes to a shell.
//
// Base64 for the same reason the surface path uses it: keystrokes are bytes, including partial
// UTF-8 sequences and control characters, and a JSON string is not a safe carrier for those.
func (a *AppAPI) SendLocalTerminalInput(id string, dataBase64 string) error {
	if a.localTerminals == nil {
		return errLocalTerminalsUnavailable
	}
	if id == "" {
		return errInvalidLocalTerminalRequest
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return errInvalidLocalTerminalRequest
	}
	return a.localTerminals.Write(id, data)
}

// ResizeLocalTerminal reports a new character grid for a shell.
func (a *AppAPI) ResizeLocalTerminal(id string, cols int, rows int) error {
	if a.localTerminals == nil {
		return errLocalTerminalsUnavailable
	}
	if id == "" {
		return errInvalidLocalTerminalRequest
	}
	if cols <= 0 || rows <= 0 || cols > maxSurfaceDimension || rows > maxSurfaceDimension {
		return errInvalidLocalTerminalRequest
	}
	return a.localTerminals.Resize(id, uint16(cols), uint16(rows))
}

// ListLocalShells reports the shells this machine has, for the settings picker.
func (a *AppAPI) ListLocalShells() ([]ShellOptionDTO, error) {
	if a.localShells == nil {
		return nil, errLocalTerminalsUnavailable
	}
	shells := a.localShells.List()
	out := make([]ShellOptionDTO, 0, len(shells))
	for _, shell := range shells {
		out = append(out, ShellOptionDTO{ID: shell.ID, Name: shell.Name})
	}
	return out, nil
}
