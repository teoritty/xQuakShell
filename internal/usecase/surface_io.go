package usecase

import (
	"fmt"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Stream names a surface.write may carry. Two values, closed set: stdout and stderr are what a
// process has, and a viewer that colours them apart cannot do so from a free-form label.
const (
	surfaceStreamStdout = "stdout"
	surfaceStreamStderr = "stderr"
)

// write handles surface.write.
//
// A write to a surface that is already gone is a no-op, not an error. The tab was closed by the
// user or by the session ending, the plugin has been told, and the bytes it had already queued
// are simply dropped — reporting that as a failure would make an ordinary race look like a fault
// in the plugin, and there is nothing it could do differently.
func (s *SurfaceService) write(pluginID string, req surfaceWriteParams) error {
	stream, err := normalizeSurfaceStream(req.Stream)
	if err != nil {
		return err
	}
	if _, err := s.registry.Get(req.SurfaceID, pluginID); err != nil {
		if isClosedSurface(s, req.SurfaceID) {
			return nil
		}
		return err
	}
	if err := s.presenter.Output(req.SurfaceID, req.DataBase64, stream); err != nil {
		// The UI consumer is behind. Same answer session.writeTerminal gives, for the same reason:
		// the plugin must slow down, and the alternative is an unbounded queue in the host.
		return fmt.Errorf("%w: surface consumer is behind", domainplugin.ErrRateLimited)
	}
	return nil
}

// updateState handles surface.updateState.
func (s *SurfaceService) updateState(pluginID string, req surfaceStateParams) error {
	if !domainplugin.ValidSurfaceState(req.State) {
		return fmt.Errorf("invalid surface state %q", req.State)
	}
	updated, err := s.registry.Update(req.SurfaceID, pluginID, func(surface *domainplugin.Surface) {
		surface.State = req.State
		surface.Error = req.Error
	})
	if err != nil {
		return err
	}
	s.presenter.Changed(updated)
	return nil
}

// setTitle handles surface.setTitle. The title goes through the same sanitizer as at open: a
// rename is not a second, gentler path into the tab bar.
func (s *SurfaceService) setTitle(pluginID string, req surfaceTitleParams) error {
	updated, err := s.registry.Update(req.SurfaceID, pluginID, func(surface *domainplugin.Surface) {
		surface.Title = sanitizeSurfaceTitle(req.Title)
	})
	if err != nil {
		return err
	}
	s.presenter.Changed(updated)
	return nil
}

func normalizeSurfaceStream(stream string) (string, error) {
	switch stream {
	case "", surfaceStreamStdout:
		return surfaceStreamStdout, nil
	case surfaceStreamStderr:
		return surfaceStreamStderr, nil
	default:
		return "", fmt.Errorf("invalid surface stream %q", stream)
	}
}

// isClosedSurface distinguishes "gone" from "someone else's" for the one caller that treats them
// differently. Get answers both with the same denial on purpose (see SurfaceRegistry.Get), so this
// asks the registry the only question it will answer plainly: is this id present at all?
//
// Note what this does NOT leak: it is consulted only after Get has already refused, and its answer
// changes a denial into a silent no-op, never the other way round. A plugin probing another
// plugin's surface id still gets the same denial it would get for an id that never existed.
func isClosedSurface(s *SurfaceService, surfaceID string) bool {
	return !s.registry.Exists(surfaceID)
}
