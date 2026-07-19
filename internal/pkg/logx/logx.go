// Package logx provides thin, component-scoped access to the process-wide
// slog logger. It adds a standard "component" attribute so log entries can be
// grouped and filtered in the debug log viewer. It introduces no logging
// backend of its own: every call flows through slog.Default(), which the
// loghub handler feeds into the hub.
package logx

import "log/slog"

// For returns a logger tagged with the given component name (for example
// "session", "sftp", "plugin.host", "plugin.channel", "rpc"). The component
// surfaces as a field the viewer renders as a badge.
func For(component string) *slog.Logger {
	return slog.Default().With("component", component)
}
