package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Surface RPC method names, in one place so the gate, the dispatcher and the tests cannot drift.
const (
	MethodSurfaceOpen        = "surface.open"
	MethodSurfaceWrite       = "surface.write"
	MethodSurfaceUpdateState = "surface.updateState"
	MethodSurfaceSetTitle    = "surface.setTitle"
	MethodSurfaceClose       = "surface.close"
)

type surfaceOpenParams struct {
	ParentSessionID string `json:"parentSessionId"`
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	IconID          string `json:"iconId,omitempty"`
}

type surfaceWriteParams struct {
	SurfaceID  string `json:"surfaceId"`
	DataBase64 string `json:"dataBase64"`
	Stream     string `json:"stream,omitempty"`
}

type surfaceStateParams struct {
	SurfaceID string `json:"surfaceId"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

type surfaceTitleParams struct {
	SurfaceID string `json:"surfaceId"`
	Title     string `json:"title"`
}

type surfaceIDParams struct {
	SurfaceID string `json:"surfaceId"`
}

// Handle dispatches one surface.* call. This file decodes and delegates and does nothing else:
// every rule about what a call is allowed to do lives in surface_open.go, surface_io.go or
// surface_lifecycle.go, so a reader looking for a rule never has to read a switch statement first.
//
// The IDOR check for surface.open (does this plugin hold the parent session?) has already run by
// the time a payload arrives here — PluginSessionRPCHandler authorizes it on the same path it
// authorizes channel.open and discovery.publish. Repeating it here would be a second copy of one
// rule, and the weaker copy is how the two drift.
func (s *SurfaceService) Handle(_ context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	case MethodSurfaceOpen:
		var req surfaceOpenParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return s.open(pluginID, req)
	case MethodSurfaceWrite:
		var req surfaceWriteParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := s.write(pluginID, req); err != nil {
			return nil, err
		}
	case MethodSurfaceUpdateState:
		var req surfaceStateParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := s.updateState(pluginID, req); err != nil {
			return nil, err
		}
	case MethodSurfaceSetTitle:
		var req surfaceTitleParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if err := s.setTitle(pluginID, req); err != nil {
			return nil, err
		}
	case MethodSurfaceClose:
		var req surfaceIDParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		s.closeOne(req.SurfaceID, pluginID, "", false)
	default:
		return nil, fmt.Errorf("%w: %s", domainplugin.ErrNotImplemented, method)
	}
	return json.Marshal(map[string]bool{"ok": true})
}
