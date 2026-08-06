package wails

import (
	"encoding/base64"
	"errors"
	"strings"
)

// errSurfacesUnavailable is what every handler returns when no surface service is wired: the vault
// may be locked, or the plugin runtime may not have been composed at all. It names no id, because
// this string reaches a user.
var errSurfacesUnavailable = errors.New("plugin surfaces unavailable")

// errInvalidSurfaceRequest covers every rejected argument. One generic message on purpose: the
// frontend is not a trust boundary and the specifics belong in the log, not in a toast.
var errInvalidSurfaceRequest = errors.New("invalid surface request")

// maxSurfaceDimension bounds a terminal geometry. The number is deliberately far above any real
// terminal and far below anything that could be mistaken for a length: cols/rows cross into a
// remote pty, and an absurd value there is a resize nobody asked for.
const maxSurfaceDimension = 10000

// SurfaceCommands is the slice of the surface use case this layer needs — the three things the UI
// can do to a surface. Named here rather than taken as *usecase.SurfaceService so a handler cannot
// reach the plugin-facing half, and so a test can hand it a recorder.
type SurfaceCommands interface {
	CloseSurfaceFromUI(surfaceID string)
	DeliverInput(surfaceID string, data []byte)
	DeliverResize(surfaceID string, cols, rows uint16)
}

// SetSurfaceService wires the surface use case. Passing nil leaves surfaces inert rather than
// half-wired.
func (a *AppAPI) SetSurfaceService(svc SurfaceCommands) {
	if a == nil {
		return
	}
	a.surfaces = svc
}

// CloseSurface closes a plugin-owned tab because the user closed it.
func (a *AppAPI) CloseSurface(surfaceId string) error {
	if a == nil || a.surfaces == nil {
		return errSurfacesUnavailable
	}
	id := strings.TrimSpace(surfaceId)
	if id == "" {
		return errInvalidSurfaceRequest
	}
	a.surfaces.CloseSurfaceFromUI(id)
	return nil
}

// SendSurfaceInput forwards keystrokes to the plugin that owns an interactive surface.
func (a *AppAPI) SendSurfaceInput(surfaceId string, dataBase64 string) error {
	if a == nil || a.surfaces == nil {
		return errSurfacesUnavailable
	}
	id := strings.TrimSpace(surfaceId)
	if id == "" {
		return errInvalidSurfaceRequest
	}
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return errInvalidSurfaceRequest
	}
	a.surfaces.DeliverInput(id, data)
	return nil
}

// ResizeSurface reports a new character grid for an interactive surface.
func (a *AppAPI) ResizeSurface(surfaceId string, cols int, rows int) error {
	if a == nil || a.surfaces == nil {
		return errSurfacesUnavailable
	}
	id := strings.TrimSpace(surfaceId)
	if id == "" {
		return errInvalidSurfaceRequest
	}
	if cols <= 0 || rows <= 0 || cols > maxSurfaceDimension || rows > maxSurfaceDimension {
		return errInvalidSurfaceRequest
	}
	a.surfaces.DeliverResize(id, uint16(cols), uint16(rows))
	return nil
}
