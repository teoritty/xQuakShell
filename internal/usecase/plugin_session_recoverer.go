package usecase

import (
	"context"
)

// PluginSessionRecoverer reconnects plugin-owned sessions after process crashes.
type PluginSessionRecoverer interface {
	RecoverPluginSession(ctx context.Context, pluginID, sessionID string) error
	FailPluginSessionRecovery(pluginID, sessionID string)
}
