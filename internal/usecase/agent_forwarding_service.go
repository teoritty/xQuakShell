package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// AgentSessionInfo names the session a forwarded agent serves, for the dialog and the audit log.
type AgentSessionInfo struct {
	SessionID      string
	ConnectionID   string
	ConnectionName string
	Host           string
	Username       string
}

// AgentSignerSource turns a vault key into a signer, asking for its passphrase when the key's own
// policy needs one. SSHConnector implements it, so a forwarded signature unlocks a key by exactly
// the rules a login does.
type AgentSignerSource interface {
	SignerFor(ctx context.Context, identityID string) (domain.Signer, error)
}

// AgentPolicyProvider hands a starting shell its agent policy.
type AgentPolicyProvider interface {
	PolicyFor(sessionCtx context.Context, sessionID, connectionID string) (domain.AgentSignPolicy, error)
}

// AgentForwardingConfig holds the dependencies of AgentForwardingService.
type AgentForwardingConfig struct {
	Connections domain.ConnectionRepository
	Identities  domain.IdentityRepository
	KnownHosts  domain.KnownHostsRepository
	Signers     AgentSignerSource
	// Confirm asks the user; nil denies every request, so a build without a dialog fails closed.
	Confirm AgentConfirmFunc
	Audit   *AgentAuditRecorder
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// AgentForwardingService owns the forwarded agents of every live session (ADR-022).
//
// It decides nothing about the agent protocol and speaks none of it. It hands each shell a policy,
// keeps the approval windows those policies grant, and forgets both when the session's context
// ends — which is what makes closing a tab, losing the connection and locking the vault (which
// closes every session) all end forwarding the same way.
type AgentForwardingService struct {
	cfg    AgentForwardingConfig
	window *agentforward.ApprovalWindow

	mu       sync.Mutex
	sessions map[string]*agentSessionPolicy
}

// NewAgentForwardingService returns a service with no live sessions.
func NewAgentForwardingService(cfg AgentForwardingConfig) *AgentForwardingService {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &AgentForwardingService{
		cfg:      cfg,
		window:   agentforward.NewApprovalWindow(cfg.Now),
		sessions: map[string]*agentSessionPolicy{},
	}
}

var _ AgentPolicyProvider = (*AgentForwardingService)(nil)

// PolicyFor returns the policy for a session about to start its shell, or nil when its connection
// forwards no agent. Plugin protocols never get one: forwarding is a property of an SSH shell to
// the final target, and a plugin connector is neither.
func (s *AgentForwardingService) PolicyFor(sessionCtx context.Context, sessionID, connectionID string) (domain.AgentSignPolicy, error) {
	conn, err := s.cfg.Connections.GetByID(sessionCtx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("agent forwarding: load connection: %w", err)
	}
	if conn.GetProtocol() != domain.ProtocolSSH || !conn.AgentForwarding.Active() {
		return nil, nil
	}
	policy := &agentSessionPolicy{
		svc: s,
		ctx: sessionCtx,
		info: AgentSessionInfo{
			SessionID: sessionID, ConnectionID: conn.ID, ConnectionName: conn.Name,
			Host: conn.Host, Username: conn.EffectiveUsername(),
		},
		limiter:     agentforward.NewRequestLimiter(agentforward.SignRequestsPerWindow, agentforward.SignRequestWindow, s.cfg.Now),
		confirmSlot: make(chan struct{}, 1),
	}
	s.register(policy)
	context.AfterFunc(sessionCtx, func() { s.forget(policy) })
	s.cfg.Audit.Record(sessionCtx, AgentAuditEvent{Event: AgentEventForwardStarted, Session: policy.info})
	return policy, nil
}

// register replaces any earlier policy for the same session id. The earlier one is then dead: its
// alive check compares identity, not id, so a reconnect cannot resurrect a policy - or the window
// grants - that belonged to the connection before it.
func (s *AgentForwardingService) register(p *agentSessionPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, existed := s.sessions[p.info.SessionID]; existed {
		s.window.ForgetSession(p.info.SessionID)
	}
	s.sessions[p.info.SessionID] = p
}

func (s *AgentForwardingService) forget(p *agentSessionPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[p.info.SessionID] != p {
		return
	}
	delete(s.sessions, p.info.SessionID)
	s.window.ForgetSession(p.info.SessionID)
}

func (s *AgentForwardingService) isLive(p *agentSessionPolicy) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[p.info.SessionID] == p
}
