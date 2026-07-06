package main

import (
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/auditlog"
	infrassh "ssh-client/internal/infra/ssh"
	"ssh-client/internal/usecase"
)

// wireSSHAuth binds usecase and infra for plugin SSH auth (composition root only).
func wireSSHAuth(pr *pluginRuntime) (*usecase.SSHAuthWiring, domainplugin.SessionAuditor) {
	if pr == nil || pr.manager == nil {
		return nil, nil
	}
	attempts := usecase.NewPluginAuthAttemptRegistry()
	return &usecase.SSHAuthWiring{
		Attempts:    attempts,
		Provider:    usecase.NewPluginAuthBridge(pr.manager, attempts),
		Builder:     infrassh.NewPluginAuthMethodBuilder(),
		Lookup:      pr.manager.Registry(),
		Starter:     pr.manager,
		GrantReader: pr.vaultSettings,
	}, auditlog.NewPluginSessionAuditLog(512)
}
