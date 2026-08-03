package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"xquakshell/internal/pkg/safego"
)

const pluginSupervisorMaxAttempts = 3

// PluginSupervisor restarts crashed plugin processes while sessions remain active.
type PluginSupervisor struct {
	manager   *PluginManager
	recoverer PluginSessionRecoverer
	gaveUp    func(pluginID string)
	mu        sync.Mutex
	inflight  map[string]struct{}
}

// NewPluginSupervisor creates a supervisor bound to the plugin manager.
func NewPluginSupervisor(manager *PluginManager) *PluginSupervisor {
	return &PluginSupervisor{
		manager:  manager,
		inflight: make(map[string]struct{}),
	}
}

// SetRecoverer binds the session manager for post-crash session.connect recovery.
func (s *PluginSupervisor) SetRecoverer(r PluginSessionRecoverer) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.recoverer = r
	s.mu.Unlock()
}

// SetGaveUpHandler binds the callback fired once the supervisor stops trying to restart a plugin.
//
// It exists because "crashed" and "abandoned" are different facts and only this type knows when the
// second one becomes true. The crash hooks on PluginManager fire on every crash, including the ones
// this supervisor is about to repair; nothing there can tell a restart in progress from a restart
// that will never come.
func (s *PluginSupervisor) SetGaveUpHandler(fn func(pluginID string)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.gaveUp = fn
	s.mu.Unlock()
}

// HandleCrash schedules exponential-backoff restart attempts when sessions are active.
func (s *PluginSupervisor) HandleCrash(pluginID, sessionID string) {
	if s == nil || s.manager == nil {
		return
	}
	// Not ActiveSessionCount: a discovery plugin holds no session of its own, and asking only about
	// sessions meant a crashed one was never restarted, never ran out of attempts, and therefore
	// never reached the give-up path that turns its stale branches into a stated failure. The whole
	// crash story below was unreachable for exactly the plugins ADR-014 describes.
	if !s.manager.PluginInUse(pluginID) {
		return
	}

	key := pluginID + "\x00" + sessionID
	s.mu.Lock()
	if _, ok := s.inflight[key]; ok {
		s.mu.Unlock()
		return
	}
	s.inflight[key] = struct{}{}
	s.mu.Unlock()

	safego.GoNamed("plugin.restartBackoff", func() { s.restartWithBackoff(pluginID, sessionID, key) })
}

func (s *PluginSupervisor) restartWithBackoff(pluginID, sessionID, key string) {
	defer func() {
		s.mu.Lock()
		delete(s.inflight, key)
		s.mu.Unlock()
	}()

	backoff := 200 * time.Millisecond
	for attempt := 1; attempt <= pluginSupervisorMaxAttempts; attempt++ {
		if !s.manager.PluginInUse(pluginID) {
			return
		}
		if !s.manager.IsPluginEnabled(pluginID) {
			slog.Info("plugin supervisor skip restart for disabled plugin", "pluginId", pluginID, "sessionId", sessionID)
			return
		}

		time.Sleep(backoff)
		backoff *= 2

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := s.manager.EnsureRunningForSession(ctx, pluginID, sessionID)
		if err != nil {
			cancel()
			slog.Warn("plugin supervisor restart failed", "pluginId", pluginID, "sessionId", sessionID, "attempt", attempt, "err", err)
			continue
		}

		params, encErr := json.Marshal(map[string]string{"reason": "crash-recovery"})
		if encErr == nil {
			if _, callErr := s.manager.CallForSession(ctx, pluginID, sessionID, "activate", params); callErr != nil {
				slog.Warn("plugin supervisor activate failed", "pluginId", pluginID, "sessionId", sessionID, "attempt", attempt, "err", callErr)
				cancel()
				continue
			}
		}

		// Session recovery runs only for a plugin that HAS a session. A shared-scope process is
		// reported as crashed with an empty sessionID (the host has no session to name), and both
		// calls below are meaningless for it: there is no session to re-authorize and none to
		// reconnect. Worse, they used to make a SUCCESSFUL restart look like a failed attempt —
		// BindSession refuses an empty sessionID with ErrSessionNotBound, the loop took the failure
		// branch, and three successful restarts in a row ended at "gave up", which now paints the
		// plugin's branches error while the plugin is running and has already refilled them. That is
		// the one thing ADR-014 forbids a branch state to do: say something the user can check and
		// find false. Trimmed, because the authorizer that refuses the id trims before checking it:
		// a guard that disagrees with the rule it is protecting against is a guard with a gap in it.
		if s.recoverer != nil && strings.TrimSpace(sessionID) != "" {
			if err := s.manager.BindSession(pluginID, sessionID); err != nil {
				slog.Warn("plugin supervisor bind session failed", "pluginId", pluginID, "sessionId", sessionID, "attempt", attempt, "err", err)
				cancel()
				continue
			}
			if err := s.recoverer.RecoverPluginSession(ctx, pluginID, sessionID); err != nil {
				slog.Warn("plugin supervisor session.connect failed", "pluginId", pluginID, "sessionId", sessionID, "attempt", attempt, "err", err)
				cancel()
				continue
			}
		}
		cancel()

		slog.Info("plugin supervisor restarted process", "pluginId", pluginID, "sessionId", sessionID, "attempt", attempt)
		return
	}

	slog.Error("plugin supervisor gave up after max attempts", "pluginId", pluginID, "sessionId", sessionID)
	// Same condition as the recovery above, and for the same reason: reporting a failed recovery of
	// a session that never existed hands the session bridge an empty id to look up.
	if s.recoverer != nil && strings.TrimSpace(sessionID) != "" {
		s.recoverer.FailPluginSessionRecovery(pluginID, sessionID)
	}
	s.notifyGaveUp(pluginID)
}

// notifyGaveUp calls the handler outside the lock: it reaches into another use case, and holding
// this supervisor's mutex across that is exactly the shape ADR-009 forbids.
func (s *PluginSupervisor) notifyGaveUp(pluginID string) {
	s.mu.Lock()
	handler := s.gaveUp
	s.mu.Unlock()
	if handler != nil {
		handler(pluginID)
	}
}
