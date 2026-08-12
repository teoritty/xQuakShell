package plugin

import (
	"fmt"
	"log/slog"

	domainplugin "xquakshell/internal/domain/plugin"
)

// fallBackOrRefuse turns a failed confinement into either an unconfined start or a refusal, and
// nothing in between.
//
// A platform that CANNOT confine never reaches here. That case is answered before anything is
// attempted — macOS, a kernel without Landlock, a Windows build without the profile API — and it
// starts the plugin unconfined with no opt-in, because a platform's inability is not a failure.
//
// This is the other case, and the two deliberately do not share a code path. A sandbox that quietly
// fell through to an unrestricted exec whenever it broke would keep reporting success while
// protecting nobody, for whichever fraction of users hit the breakage — which is the failure mode
// the whole status pipeline (#79) was built to make impossible. Refusing is loud, diagnosable, and
// recoverable by a setting the user has to choose deliberately.
//
// The fallback is logged at warning level every single time. A user who turned the setting on
// accepted a risk; they did not ask to stop being told about it.
func fallBackOrRefuse(req childRequest, cause error) (startedChild, error) {
	if !req.policy.AllowUnsandboxedFallback {
		return startedChild{}, fmt.Errorf("%w for plugin %s: %w",
			domainplugin.ErrSandboxUnavailable, req.plugin.Manifest.ID, cause)
	}
	slog.Warn("plugin sandbox could not be applied; starting the plugin unconfined because the "+
		"fallback setting is enabled",
		"pluginId", req.plugin.Manifest.ID, "sessionId", req.sessionID, "err", cause)

	target := spawnTarget{path: req.entryPath}
	return startExecChild(target, req, domainplugin.SandboxDisabled)
}
