package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/safego"
)

// SessionLifecycleService orchestrates session open, close, retry, and SSH connect.
//
// WHY THIS FILE/TYPE EXISTS (see ADR-009, docs/adr/009-session-manager-decomposition.md):
// Session lifecycle coordinates registry, SSH connector, IO, plugin bridge, and
// embed services. It must not implement PTY, plugin RPC, or embed protocol details.
type SessionLifecycleService struct {
	registry        *SessionRegistry
	connRepo        domain.ConnectionRepository
	sshConnector    *SSHConnector
	io              *SessionIOService
	plugins         *PluginSessionBridge
	embed           *EmbedTunnelService
	channelBus      domainplugin.ChannelSessionCloser
	surfaces        domainplugin.SurfaceSessionCloser
	discovery       DiscoverySessionTracker
	dynamicForward  *DynamicForwardCoordinator
	forwardRules    *ForwardRuleValidator
	forwardLimiter  func() domain.ConcurrencyLimiter
	passphraseCache domain.PassphraseCache
	onStateChange   StateChangeFunc
	hostKeyRequest  HostKeyRequestFunc
}

type SessionLifecycleConfig struct {
	Registry                  *SessionRegistry
	ConnRepo                  domain.ConnectionRepository
	SSHConnector              *SSHConnector
	Plugins                   *PluginSessionBridge
	PassphraseCache           domain.PassphraseCache
	DynamicForward            *DynamicForwardCoordinator
	ForwardRules              *ForwardRuleValidator
	ForwardConnLimiterFactory func() domain.ConcurrencyLimiter
	OnStateChange             StateChangeFunc
	HostKeyRequest            HostKeyRequestFunc
}

func NewSessionLifecycleService(cfg SessionLifecycleConfig) *SessionLifecycleService {
	return &SessionLifecycleService{
		registry:        cfg.Registry,
		connRepo:        cfg.ConnRepo,
		sshConnector:    cfg.SSHConnector,
		plugins:         cfg.Plugins,
		passphraseCache: cfg.PassphraseCache,
		dynamicForward:  cfg.DynamicForward,
		forwardRules:    cfg.ForwardRules,
		forwardLimiter:  cfg.ForwardConnLimiterFactory,
		onStateChange:   cfg.OnStateChange,
		hostKeyRequest:  cfg.HostKeyRequest,
	}
}

func (s *SessionLifecycleService) SetIO(io *SessionIOService) {
	s.io = io
}

func (s *SessionLifecycleService) SetEmbed(embed *EmbedTunnelService) {
	s.embed = embed
}

func (s *SessionLifecycleService) SetChannelBus(bus domainplugin.ChannelSessionCloser) {
	s.channelBus = bus
}

func (s *SessionLifecycleService) SetDiscovery(tracker DiscoverySessionTracker) {
	s.discovery = tracker
}

// SetSurfaces wires the plugin surface closer, late-bound exactly like SetChannelBus: the surface
// service is built after this one in the composition root, and both are optional in tests.
func (s *SessionLifecycleService) SetSurfaces(closer domainplugin.SurfaceSessionCloser) {
	s.surfaces = closer
}

// OpenSession creates a new session for the given connection ID.
func (s *SessionLifecycleService) OpenSession(ctx context.Context, connectionID string) (string, error) {
	conn, err := s.connRepo.GetByID(ctx, connectionID)
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	if err := conn.ValidateForConnect(); err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}

	proto := conn.GetProtocol()
	sessionID := newRandomID()
	sessionCtx, cancel := context.WithCancel(context.Background())

	entry := newSessionEntry(domain.ConnectionSession{
		SessionID:      sessionID,
		ConnectionID:   connectionID,
		ConnectionName: conn.Name,
		Protocol:       proto,
		State:          domain.SessionConnecting,
	}, sessionCtx, cancel, connectionID)

	s.registry.Put(sessionID, entry)
	s.notifyStateChange(entry.info)

	if proto == domain.ProtocolSSH {
		safego.GoNamed("session.connect", func() { s.connectSession(entry, conn) })
	} else if s.plugins != nil && s.plugins.SupportsProtocol(proto) {
		safego.GoNamed("session.plugin", func() { s.plugins.RunSession(sessionID, conn) })
	} else {
		s.updateState(entry, domain.SessionError, fmt.Sprintf("protocol %s not yet implemented", proto))
	}
	return sessionID, nil
}

// CloseSession terminates a session by its ID, releasing all resources.
func (s *SessionLifecycleService) CloseSession(sessionID string) error {
	entry, ok := s.registry.Delete(sessionID)
	if !ok {
		return domain.ErrSessionNotFound
	}

	entry.cancel()
	entry.readyOnce.Do(func() { close(entry.readyCh) })

	if s.embed != nil {
		_ = s.embed.RevokeBySession(sessionID)
	}
	if entry.pluginID != "" && s.plugins != nil {
		s.plugins.Disconnect(context.Background(), entry.pluginID, sessionID)
	}
	if entry.pluginOutput != nil {
		close(entry.pluginOutput)
	}
	if entry.ptyBridge != nil {
		if err := entry.ptyBridge.Close(); err != nil {
			slog.Warn("close pty bridge failed", "sessionID", sessionID, "err", err)
		}
	}
	if entry.remoteFS != nil {
		if err := entry.remoteFS.Close(); err != nil {
			slog.Warn("close remote fs failed", "sessionID", sessionID, "err", err)
		}
	}
	if s.dynamicForward != nil {
		s.dynamicForward.StopSession(sessionID)
	}
	if entry.forwardRunner != nil {
		entry.forwardRunner.StopAll()
	}
	if s.channelBus != nil {
		// Must run before sshClient.Close(): exec-purpose channels ride this session's ssh
		// client, so closing the client first would sever them uncleanly instead of letting
		// each backend tear down its own remote end via CloseRemote.
		s.channelBus.CloseSession(sessionID)
	}
	if s.surfaces != nil {
		// Alongside the channels, and for the same reason: a surface is a view onto work this
		// session authorized, so it must not outlive the session. It runs after the channels
		// because a surface is usually fed BY one — closing the producer first means the tab is
		// removed with nothing still trying to write to it.
		s.surfaces.CloseSurfacesForSession(sessionID)
	}
	if s.discovery != nil {
		// Discovery outlives this session whenever another ready one remains — the tree belongs to
		// the connection, not the tab — so this reports a departure and lets the leader decide
		// between a handover and a teardown.
		s.discovery.SessionClosed(sessionID, entry.connectionID)
	}
	if entry.sshClient != nil {
		if err := entry.sshClient.Close(); err != nil {
			slog.Warn("close ssh client failed", "sessionID", sessionID, "err", err)
		}
	}

	entry.info.State = domain.SessionClosed
	s.notifyStateChange(entry.info)
	return nil
}

func (s *SessionLifecycleService) CloseAll() {
	for _, id := range s.registry.IDs() {
		s.CloseSession(id)
	}
	if s.passphraseCache != nil {
		s.passphraseCache.Clear()
	}
}

func (s *SessionLifecycleService) GetState(sessionID string) (domain.ConnectionSession, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return domain.ConnectionSession{}, domain.ErrSessionNotFound
	}
	return entry.info, nil
}

func (s *SessionLifecycleService) GetAllSessions() []domain.ConnectionSession {
	entries := s.registry.All()
	result := make([]domain.ConnectionSession, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.info)
	}
	return result
}

// RetrySession re-attempts the SSH connection for a session in hostkey-required state.
func (s *SessionLifecycleService) RetrySession(ctx context.Context, sessionID string) error {
	var info domain.ConnectionSession
	var connID string
	transitioned := s.registry.CompareAndTransition(
		sessionID,
		domain.SessionHostKeyRequired, domain.SessionConnecting,
		func(e *sessionEntry) {
			e.info.ErrorMessage = ""
			e.hostKeyInfo = nil
			info = e.info
			connID = e.connectionID
		},
	)
	if !transitioned {
		if _, ok := s.registry.Get(sessionID); !ok {
			return domain.ErrSessionNotFound
		}
		return fmt.Errorf("session %s not in hostkey-required state", sessionID)
	}
	s.notifyStateChange(info)

	entry, _ := s.registry.Get(sessionID)
	conn, err := s.connRepo.GetByID(ctx, connID)
	if err != nil {
		slog.Error("retry session: load connection failed", "sessionID", sessionID, "err", err)
		s.updateState(entry, domain.SessionError, "Connection not found")
		return nil
	}
	if err := conn.ValidateForConnect(); err != nil {
		slog.Error("retry session: invalid connection", "sessionID", sessionID, "err", err)
		s.updateState(entry, domain.SessionError, "Invalid connection configuration")
		return nil
	}
	safego.GoNamed("session.reconnect", func() { s.connectSession(entry, conn) })
	return nil
}

// NotifySessionDisconnected updates state when the SSH connection is lost.
func (s *SessionLifecycleService) NotifySessionDisconnected(sessionID string) {
	entry, ok := s.registry.Get(sessionID)
	if !ok || entry.info.State != domain.SessionReady {
		return
	}
	s.updateState(entry, domain.SessionError, "Connection lost")
}

func (s *SessionLifecycleService) GetHostKeyInfo(sessionID string) (*domain.HostKeyInfo, error) {
	entry, ok := s.registry.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.hostKeyInfo, nil
}

func (s *SessionLifecycleService) SetForwardRuleValidator(v *ForwardRuleValidator) {
	if s != nil {
		s.forwardRules = v
	}
}

func (s *SessionLifecycleService) connectSession(entry *sessionEntry, conn *domain.Connection) {
	for _, rule := range conn.ForwardRules {
		if !rule.Enabled {
			continue
		}
		if s.forwardRules != nil {
			if err := s.forwardRules.ValidateRuleForConnect(rule); err != nil {
				msg := fmt.Sprintf(`Forward rule "%s" invalid: %v`, rule.ID, err)
				slog.Warn("forward rule connect validation failed", "ruleId", rule.ID, "err", err)
				s.updateState(entry, domain.SessionError, msg)
				return
			}
			continue
		}
		if err := rule.Validate(); err != nil {
			msg := fmt.Sprintf(`Forward rule "%s" invalid: %v`, rule.ID, err)
			slog.Warn("forward rule connect validation failed", "ruleId", rule.ID, "err", err)
			s.updateState(entry, domain.SessionError, msg)
			return
		}
	}

	slog.Info("session connecting", "component", "session", "sessionID", entry.info.SessionID, "connectionId", conn.ID, "host", conn.EffectiveHost())
	result := s.sshConnector.Connect(entry.ctx, conn)
	if result.HostKeyInfo != nil {
		s.applyHostKeyRequired(entry, *result.HostKeyInfo, result.Err)
		return
	}
	if result.Err != nil {
		slog.Error("session connect failed", "sessionID", entry.info.SessionID, "err", result.Err)
		s.updateState(entry, domain.SessionError, sshConnectErrorMessage(result.Err))
		return
	}

	slog.Info("session connected", "component", "session", "sessionID", entry.info.SessionID, "connectionId", conn.ID)
	s.registry.Mutate(entry.info.SessionID, func(e *sessionEntry) {
		e.sshClient = result.Client
		var limiter domain.ConcurrencyLimiter
		if s.forwardLimiter != nil {
			limiter = s.forwardLimiter()
		}
		e.forwardRunner = NewForwardRuleRunner(result.Client, limiter)
	})
	for _, rule := range conn.ForwardRules {
		if !rule.Enabled {
			continue
		}
		if rule.Kind == domain.ForwardRuleDynamic {
			continue
		}
		if err := rule.Validate(); err != nil {
			slog.Warn("skip invalid forward rule", "ruleId", rule.ID, "err", err)
			continue
		}
		entry, _ := s.registry.Get(entry.info.SessionID)
		if entry == nil || entry.forwardRunner == nil {
			break
		}
		if err := entry.forwardRunner.Start(entry.ctx, rule); err != nil {
			slog.Warn("forward rule start failed", "ruleId", rule.ID, "err", err)
		}
	}
	if s.dynamicForward != nil {
		s.dynamicForward.StartSession(entry.ctx, entry.info.SessionID, result.Client, conn.ForwardRules)
	}
	if result.JumpCleanup != nil {
		safego.GoNamed("session.jumpCleanup", func() {
			<-entry.ctx.Done()
			result.JumpCleanup()
		})
	}
	if s.io != nil {
		safego.GoNamed("session.serverAlive", func() { s.io.RunServerAlive(entry) })
	}
	s.updateState(entry, domain.SessionReady, "")
}

func (s *SessionLifecycleService) applyHostKeyRequired(entry *sessionEntry, hkInfo domain.HostKeyInfo, err error) {
	s.registry.Mutate(entry.info.SessionID, func(e *sessionEntry) {
		e.hostKeyInfo = &hkInfo
	})
	msg := "Host key verification required"
	if hkInfo.Mismatch || errors.Is(err, domain.ErrHostKeyMismatch) {
		msg = "Host key mismatch"
	}
	s.updateState(entry, domain.SessionHostKeyRequired, msg)
	if s.hostKeyRequest != nil {
		s.hostKeyRequest(entry.info.SessionID, hkInfo)
	}
}

func (s *SessionLifecycleService) updateState(entry *sessionEntry, state domain.SessionState, errMsg string) {
	sessionID := entry.info.SessionID
	var info domain.ConnectionSession
	if !s.registry.Mutate(sessionID, func(e *sessionEntry) {
		e.info.State = state
		e.info.ErrorMessage = errMsg
		info = e.info
		e.signalReadyIfTerminal(state)
	}) {
		return
	}
	if s.discovery != nil {
		// Discovery cares about exactly one distinction: can this session still carry an
		// enumeration, or not. Leaving ready is the same event as closing — there is no third case
		// worth a branch here, and inventing one is how the two paths drift apart.
		if state == domain.SessionReady {
			s.discovery.SessionReady(sessionID, entry.connectionID)
		} else {
			s.discovery.SessionClosed(sessionID, entry.connectionID)
		}
	}
	s.notifyStateChange(info)
}

func (s *SessionLifecycleService) notifyStateChange(info domain.ConnectionSession) {
	if s.onStateChange != nil {
		s.onStateChange(info)
	}
}

func sshConnectErrorMessage(err error) string {
	if err == nil {
		return "Connection failed"
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "authentication failed:"):
		return "Authentication failed"
	case strings.HasPrefix(msg, "jump chain connection failed:"):
		return "Jump chain connection failed"
	default:
		return "Connection failed"
	}
}
