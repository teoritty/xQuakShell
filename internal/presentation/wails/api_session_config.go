package wails

import "xquakshell/internal/usecase"

// applySSHAuth hands plugin-provided SSH authentication to the session manager when a build has it.
func applySSHAuth(cfg *usecase.SessionManagerConfig, sshAuth *usecase.SSHAuthWiring) {
	if sshAuth == nil || !sshAuth.Enabled() {
		return
	}
	cfg.AuthProvider = sshAuth.Provider
	cfg.AuthMethodBuilder = sshAuth.Builder
	cfg.AuthAttempts = sshAuth.Attempts
	cfg.AuthLookup = sshAuth.Lookup
	cfg.AuthStarter = sshAuth.Starter
	cfg.AuthGrantReader = sshAuth.GrantReader
}
