package host

import "ssh-client/internal/domain"

// AppLauncher opens files with the system default app or a specified editor.
type AppLauncher struct{}

// NewAppLauncher creates a host app launcher.
func NewAppLauncher() *AppLauncher {
	return &AppLauncher{}
}

var _ domain.HostAppLauncher = (*AppLauncher)(nil)
