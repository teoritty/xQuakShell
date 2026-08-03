package plugin

import (
	"context"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Stop gracefully shuts down a plugin process.
func (h *ProcessHost) Stop(ctx context.Context, pluginID, sessionID string) error {
	key := h.resolveKey(pluginID, sessionID)
	h.mu.Lock()
	mp, ok := h.processes[key]
	if !ok {
		h.mu.Unlock()
		return nil
	}
	mp.state = domainplugin.ProcessStopping
	h.mu.Unlock()

	if mp.conn != nil {
		_ = mp.conn.Notify("deactivate", nil)

		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownCallTimeout)
		_, _ = mp.conn.Call(shutdownCtx, "shutdown", nil)
		shutdownCancel()

		mp.conn.CloseWrite()
	}

	if mp.reaper != nil {
		waitCtx, waitCancel := context.WithTimeout(ctx, stopGracePeriod)
		waitErr := mp.reaper.Wait(waitCtx)
		waitCancel()
		if waitErr != nil {
			_ = mp.reaper.Kill()
		}
	} else if mp.cmd != nil && mp.cmd.Process != nil {
		_ = mp.cmd.Process.Kill()
	}

	h.finalizeProcess(key, mp)
	return nil
}

// StopAll stops every running plugin (app shutdown).
func (h *ProcessHost) StopAll(ctx context.Context) {
	h.mu.Lock()
	targets := make([]struct {
		pluginID  string
		sessionID string
	}, 0, len(h.processes))
	for _, mp := range h.processes {
		targets = append(targets, struct {
			pluginID  string
			sessionID string
		}{mp.plugin.Manifest.ID, mp.sessionID})
	}
	h.mu.Unlock()

	for _, target := range targets {
		if err := h.Stop(ctx, target.pluginID, target.sessionID); err != nil {
			slog.Warn("stop plugin failed", "pluginId", target.pluginID, "err", err)
		}
	}
	KillAllTrackedPlugins()
}
