package plugin

import (
	"fmt"
	"strings"
)

// SurfaceKind is the closed set of plugin-owned tab kinds (ADR-015).
type SurfaceKind string

const (
	// SurfaceKindTerminal is a duplex byte stream rendered by the terminal emulator: the user
	// types, the plugin receives surface.input, and a resize is a real cols/rows event the far end
	// can act on.
	SurfaceKindTerminal SurfaceKind = "terminal"
	// SurfaceKindLog is a one-way stream rendered by a viewer the core owns, with search,
	// stdout/stderr distinction and export. None of those are things a terminal emulator can
	// offer, because by the time bytes reach it they are already screen cells.
	SurfaceKindLog SurfaceKind = "log"
)

// ParseSurfaceKind converts a wire value to a SurfaceKind. An unknown kind is an error rather than
// a default: the two kinds behave differently enough that guessing would produce a tab the user
// cannot use for the thing it was opened for.
func ParseSurfaceKind(s string) (SurfaceKind, error) {
	switch SurfaceKind(strings.TrimSpace(s)) {
	case SurfaceKindTerminal:
		return SurfaceKindTerminal, nil
	case SurfaceKindLog:
		return SurfaceKindLog, nil
	default:
		return "", fmt.Errorf("unknown surface kind %q", s)
	}
}

// Interactive reports whether this kind accepts input from the user. Callers use it to decide
// whether surface.input and surface.resize are meaningful for a surface, rather than comparing
// against SurfaceKindTerminal in five places.
func (k SurfaceKind) Interactive() bool { return k == SurfaceKindTerminal }
