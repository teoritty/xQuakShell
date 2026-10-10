package usecase

// newAgentForwarding builds the forwarded-agent service for a SessionManager, or nil when the
// build supplies no confirmation dialog. Without a dialog no request could ever be approved, so
// offering the agent at all would only show the server keys it can never use.
func newAgentForwarding(cfg SessionManagerConfig, signers AgentSignerSource) AgentPolicyProvider {
	if cfg.AgentConfirm == nil {
		return nil
	}
	return NewAgentForwardingService(AgentForwardingConfig{
		Connections: cfg.ConnRepo,
		Identities:  cfg.IdentRepo,
		KnownHosts:  cfg.KnownHosts,
		Signers:     signers,
		Confirm:     cfg.AgentConfirm,
		Audit:       NewAgentAuditRecorder(cfg.AgentAudit),
	})
}
