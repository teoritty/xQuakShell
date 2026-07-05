package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

const embedSandboxDefault = "allow-scripts allow-same-origin"

// PluginTunnelNotifier sends tunnel data and control notifications to a plugin process.
type PluginTunnelNotifier func(ctx context.Context, pluginID, sessionID, method string, params []byte) error

// EmbedReadyFunc is invoked when an embed descriptor becomes available for a session.
type EmbedReadyFunc func(desc domain.SessionEmbedDescriptor)

// EmbedTunnelService manages embed token lifecycle and tunnel frame routing.
//
// WHY embed protocol handlers live here and not in SessionManager (ADR-009):
// Embed token minting, manifest validation, and tunnel frame routing are embed-
// subsystem concerns. SessionManager only delegates; this service owns session
// registry lookups and plugin manifest checks for embed RPC.
type EmbedTunnelService struct {
	mu sync.RWMutex

	entries        map[string]*embedEntry // token -> entry
	sessionToken   map[string]string      // sessionID -> active token
	sessionActive  map[string]bool
	limiterFactory domain.RateLimiterFactory
	notifyPlugin   PluginTunnelNotifier
	onEmbedReady   EmbedReadyFunc

	registry         *SessionRegistry
	manifestLookup   PluginManifestLookup
}

type embedEntry struct {
	reg        domain.EmbedRegistration
	tunnelOpen map[string]bool
	limiter    domain.RateLimiter
	wsConns    map[string]*embedWSConn
	active     bool
}

type embedWSConn struct {
	send chan []byte
	done chan struct{}
}

// Send returns the outbound frame channel for browser delivery.
func (c *embedWSConn) Send() <-chan []byte {
	if c == nil {
		return nil
	}
	return c.send
}

// Done signals WebSocket teardown.
func (c *embedWSConn) Done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.done
}

// NewEmbedTunnelService creates an embed tunnel registry and broker coordinator.
func NewEmbedTunnelService(factory domain.RateLimiterFactory) *EmbedTunnelService {
	if factory == nil {
		panic("usecase: EmbedTunnelService requires RateLimiterFactory")
	}
	return &EmbedTunnelService{
		entries:        make(map[string]*embedEntry),
		sessionToken:   make(map[string]string),
		sessionActive:  make(map[string]bool),
		limiterFactory: factory,
	}
}

// SetPluginNotifier wires host→plugin tunnel notifications.
func (s *EmbedTunnelService) SetPluginNotifier(fn PluginTunnelNotifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifyPlugin = fn
}

// SetEmbedReadyHandler wires embed-ready callbacks to the presentation layer.
func (s *EmbedTunnelService) SetEmbedReadyHandler(fn EmbedReadyFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEmbedReady = fn
}

// WireSessionContext binds session registry and manifest lookup for plugin RPC handlers.
func (s *EmbedTunnelService) WireSessionContext(registry *SessionRegistry, lookup PluginManifestLookup) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.registry = registry
	s.manifestLookup = lookup
	s.mu.Unlock()
}

// Register mints a new embed token for a session (replaces any prior token).
func (s *EmbedTunnelService) Register(ctx context.Context, reg domain.EmbedRegistration) (domain.SessionEmbedDescriptor, error) {
	if s == nil {
		return domain.SessionEmbedDescriptor{}, fmt.Errorf("embed tunnel service unavailable")
	}
	if reg.SessionID == "" || reg.PluginID == "" {
		return domain.SessionEmbedDescriptor{}, domainplugin.ErrInvalidManifest
	}
	if len(reg.TunnelIDs) == 0 {
		reg.TunnelIDs = []string{"main"}
	}
	if len(reg.TunnelIDs) > domain.MaxTunnelsPerSession {
		return domain.SessionEmbedDescriptor{}, fmt.Errorf("too many tunnels")
	}
	token, err := mintEmbedToken()
	if err != nil {
		return domain.SessionEmbedDescriptor{}, err
	}
	if reg.ExpiresAt.IsZero() {
		reg.ExpiresAt = time.Now().Add(domain.DefaultEmbedTokenTTL)
	}
	reg.Token = token

	s.mu.Lock()
	if oldToken, ok := s.sessionToken[reg.SessionID]; ok {
		delete(s.entries, oldToken)
	}
	entry := &embedEntry{
		reg:        reg,
		tunnelOpen: make(map[string]bool),
		limiter: s.limiterFactory.New(
			domain.DefaultTunnelBandwidthBytesPerSec,
			domain.MaxTunnelFrameSize,
		),
		wsConns: make(map[string]*embedWSConn),
		active:  true,
	}
	if v, ok := s.sessionActive[reg.SessionID]; ok {
		entry.active = v
	}
	for _, id := range reg.TunnelIDs {
		entry.tunnelOpen[id] = false
	}
	s.entries[token] = entry
	s.sessionToken[reg.SessionID] = token
	ready := s.onEmbedReady
	s.mu.Unlock()

	mainTunnel := reg.TunnelIDs[0]
	desc := domain.SessionEmbedDescriptor{
		SessionID: reg.SessionID,
		PluginID:  reg.PluginID,
		UIUrl:     embedUIPath(token, reg.UIEntry),
		TunnelUrl: embedTunnelPath(token, mainTunnel),
		Sandbox:   []string{"allow-scripts", "allow-same-origin"},
		ExpiresAt: reg.ExpiresAt,
	}
	if ready != nil {
		ready(desc)
	}
	_ = ctx
	return desc, nil
}

// RevokeBySession invalidates the embed token for a session.
func (s *EmbedTunnelService) RevokeBySession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.sessionToken[sessionID]
	if !ok {
		return nil
	}
	if entry, ok := s.entries[token]; ok {
		entry.reg.Revoked = true
		for _, conn := range entry.wsConns {
			close(conn.done)
		}
	}
	delete(s.sessionToken, sessionID)
	delete(s.entries, token)
	delete(s.sessionActive, sessionID)
	return nil
}

// Lookup returns registration data for a token.
func (s *EmbedTunnelService) Lookup(token string) (domain.EmbedRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[token]
	if !ok || entry.reg.Revoked {
		return domain.EmbedRegistration{}, domain.ErrSessionNotFound
	}
	if time.Now().After(entry.reg.ExpiresAt) {
		return domain.EmbedRegistration{}, domain.ErrSessionNotFound
	}
	return entry.reg, nil
}

// GetDescriptor returns the current embed descriptor for a session.
func (s *EmbedTunnelService) GetDescriptor(sessionID string) (domain.SessionEmbedDescriptor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.sessionToken[sessionID]
	if !ok {
		return domain.SessionEmbedDescriptor{}, false
	}
	entry, ok := s.entries[token]
	if !ok || entry.reg.Revoked {
		return domain.SessionEmbedDescriptor{}, false
	}
	mainTunnel := entry.reg.TunnelIDs[0]
	return domain.SessionEmbedDescriptor{
		SessionID: sessionID,
		PluginID:  entry.reg.PluginID,
		UIUrl:     embedUIPath(token, entry.reg.UIEntry),
		TunnelUrl: embedTunnelPath(token, mainTunnel),
		Sandbox:   []string{"allow-scripts", "allow-same-origin"},
		ExpiresAt: entry.reg.ExpiresAt,
	}, true
}

// OpenTunnel marks a tunnel as open for a session.
func (s *EmbedTunnelService) OpenTunnel(sessionID, tunnelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.entryForSession(sessionID)
	if err != nil {
		return err
	}
	if _, ok := entry.tunnelOpen[tunnelID]; !ok {
		return domainplugin.ErrHandleNotFound
	}
	entry.tunnelOpen[tunnelID] = true
	return nil
}

// RouteTunnelFrameFromPlugin forwards plugin bytes to the browser WebSocket.
func (s *EmbedTunnelService) RouteTunnelFrameFromPlugin(_ context.Context, sessionID, tunnelID string, data []byte) error {
	if len(data) > domain.MaxTunnelFrameSize {
		return domainplugin.ErrRateLimited
	}
	s.mu.RLock()
	entry, err := s.entryForSessionLocked(sessionID)
	if err != nil {
		s.mu.RUnlock()
		return err
	}
	if !entry.active {
		s.mu.RUnlock()
		return domainplugin.ErrTerminalBackpressure
	}
	if !entry.limiter.AllowN(len(data)) {
		s.mu.RUnlock()
		return domainplugin.ErrRateLimited
	}
	conn := entry.wsConns[tunnelID]
	s.mu.RUnlock()
	if conn == nil {
		return nil
	}
	select {
	case conn.send <- data:
		return nil
	default:
		return domainplugin.ErrTerminalBackpressure
	}
}

// RouteTunnelFrameToPlugin forwards browser WebSocket bytes to the plugin.
func (s *EmbedTunnelService) RouteTunnelFrameToPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error {
	s.mu.RLock()
	entry, err := s.entryForSessionLocked(sessionID)
	notify := s.notifyPlugin
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if !entry.active {
		return nil
	}
	if notify == nil {
		return nil
	}
	payload, err := encodeTunnelData(sessionID, tunnelID, data)
	if err != nil {
		return err
	}
	return notify(ctx, entry.reg.PluginID, sessionID, "session.tunnelData", payload)
}

// CloseTunnel marks a tunnel closed for a session.
func (s *EmbedTunnelService) CloseTunnel(sessionID, tunnelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, err := s.entryForSession(sessionID)
	if err != nil {
		return err
	}
	if _, ok := entry.tunnelOpen[tunnelID]; ok {
		entry.tunnelOpen[tunnelID] = false
	}
	if conn, ok := entry.wsConns[tunnelID]; ok {
		close(conn.done)
		delete(entry.wsConns, tunnelID)
	}
	return nil
}

// SetSessionActive updates tab-active state and broker backpressure.
func (s *EmbedTunnelService) SetSessionActive(sessionID string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionActive[sessionID] = active
	token, ok := s.sessionToken[sessionID]
	if !ok {
		return
	}
	if entry, ok := s.entries[token]; ok {
		entry.active = active
	}
}

// AttachWebSocket registers a browser tunnel consumer.
func (s *EmbedTunnelService) AttachWebSocket(token, tunnelID string) (domain.EmbedTunnelStream, domain.EmbedRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[token]
	if !ok || entry.reg.Revoked || time.Now().After(entry.reg.ExpiresAt) {
		return nil, domain.EmbedRegistration{}, domain.ErrSessionNotFound
	}
	if _, ok := entry.tunnelOpen[tunnelID]; !ok {
		return nil, domain.EmbedRegistration{}, domainplugin.ErrHandleNotFound
	}
	if old, ok := entry.wsConns[tunnelID]; ok {
		close(old.done)
	}
	conn := &embedWSConn{
		send: make(chan []byte, 256),
		done: make(chan struct{}),
	}
	entry.wsConns[tunnelID] = conn
	entry.tunnelOpen[tunnelID] = true
	entry.active = s.sessionActive[entry.reg.SessionID]
	return conn, entry.reg, nil
}

// UIEntryForToken returns the ui entry path for a token.
func (s *EmbedTunnelService) UIEntryForToken(token string) (string, error) {
	reg, err := s.Lookup(token)
	if err != nil {
		return "", err
	}
	return reg.UIEntry, nil
}

func (s *EmbedTunnelService) entryForSession(sessionID string) (*embedEntry, error) {
	token, ok := s.sessionToken[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	entry, ok := s.entries[token]
	if !ok || entry.reg.Revoked {
		return nil, domain.ErrSessionNotFound
	}
	return entry, nil
}

func (s *EmbedTunnelService) entryForSessionLocked(sessionID string) (*embedEntry, error) {
	token, ok := s.sessionToken[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	entry, ok := s.entries[token]
	if !ok || entry.reg.Revoked {
		return nil, domain.ErrSessionNotFound
	}
	return entry, nil
}

func mintEmbedToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func embedUIPath(token, uiEntry string) string {
	_ = uiEntry
	return fmt.Sprintf("/embed/s/%s/ui/index.html", token)
}

func embedTunnelPath(token, tunnelID string) string {
	return fmt.Sprintf("/embed/s/%s/tunnel/%s", token, tunnelID)
}

func extractEmbedToken(uiURL string) string {
	const prefix = "/embed/s/"
	idx := strings.Index(uiURL, prefix)
	if idx < 0 {
		return ""
	}
	rest := uiURL[idx+len(prefix):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

var (
	_ domain.EmbedTunnelRegistry = (*EmbedTunnelService)(nil)
	_ domain.EmbedTunnelPort     = (*EmbedTunnelService)(nil)
)
