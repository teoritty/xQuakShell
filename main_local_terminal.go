package main

import (
	"xquakshell/internal/domain"
	"xquakshell/internal/infra/localshell"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// wireLocalTerminal assembles the local shell feature and hands it to the Wails API.
//
// Assembled here rather than inline in composeApp because it is one subject, and because what it
// borrows from the rest of the application is short enough to be worth stating plainly: the audit
// log, and a way to read the current settings. It reaches nothing in the plugin runtime, and the
// plugin runtime reaches nothing here - which is the composition-root half of the rule that a
// plugin must never be able to start a shell.
func wireLocalTerminal(api *presentation.AppAPI, auditLog domain.AuditLogRepository) {
	settings := api.SettingsService()
	catalog := localshell.NewCatalog()
	svc := usecase.NewLocalTerminalService(usecase.LocalTerminalServiceConfig{
		Factory:   localshell.NewFactory(catalog),
		Catalog:   catalog,
		Presenter: presentation.NewLocalTerminalPresenter(api),
		Auditor:   usecase.NewLocalTerminalAuditRecorder(auditLog),
		// Read at open time, not captured: a shell picked in settings applies to the next
		// terminal without anything having to be invalidated or re-wired.
		ShellID: func() string { return storedShellID(settings) },
	})
	api.SetLocalTerminalService(svc)
}

// storedShellID reads the chosen shell, tolerating a settings service that cannot answer.
//
// An empty result is a valid answer and not an error: it means "this platform's default", which
// is exactly what a locked or unreadable vault should fall back to. A terminal must still open
// for a user who has not unlocked anything yet.
func storedShellID(settings *usecase.SettingsService) string {
	if settings == nil {
		return ""
	}
	current, err := settings.GetSettings()
	if err != nil {
		return ""
	}
	return current.LocalTerminal.ShellID
}
