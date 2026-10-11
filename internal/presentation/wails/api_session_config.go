package wails

import (
	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

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

// applyAgentForwarding gives the session manager the dialog that confirms forwarded-agent
// signatures (ADR-022). Without it the session manager offers no agent at all.
func (a *AppAPI) applyAgentForwarding(cfg *usecase.SessionManagerConfig, audit domain.AuditLogRepository) {
	cfg.AgentConfirm = a.onAgentConfirm
	cfg.AgentAudit = audit
}
