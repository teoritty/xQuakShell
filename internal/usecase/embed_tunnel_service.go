package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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

	// baseURL, when set, is prepended to the (otherwise root-relative) embed UI/tunnel paths so
	// the descriptor carries an absolute origin. On Windows/WebView2 the Wails asset server
	// (wails.localhost) serves HTTP but does NOT proxy WebSocket upgrades, so the embed tunnel
	// must resolve to a real loopback listener (http://127.0.0.1:<port>) that serves both the UI
	// assets and the ws:// tunnel from one same-origin host. Empty keeps the legacy relative URLs.
	baseURL string

	// outbound holds the embed-stream channel subscribers, sessionID -> tunnelID -> sub. It is
	// deliberately keyed on the session rather than stored on embedEntry: a subscription outlives a
	// token rotation (Register replaces the entry), and D1's routing predicate is defined per
	// (sessionID, tunnelID), not per token.
	outbound map[string]map[string]*embedOutboundSub
}

// embedOutboundSub is one channel-bus subscriber for a (sessionID, tunnelID) pair's browser->plugin
// input.
//
// ch is UNBUFFERED, on purpose. This direction is control input: every event is a state
// transition, not a snapshot, so it may never be dropped (readiness 7.3), and the channel bus below
// it now has real credit-based backpressure to propagate. A buffer here would absorb that
// backpressure into an unbounded host-side queue -- the very thing ADR-011's credit window exists
// to remove -- and a bounded buffer would eventually have to drop. Blocking does neither: the
// browser's WebSocket read loop simply stops reading until the plugin's window reopens.
//
// done, not close(ch), signals teardown: senders hold no reference to the unsubscriber, so closing
// ch would race a send into a panic.
type embedOutboundSub struct {
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

func (sub *embedOutboundSub) stop() {
	sub.once.Do(func() { close(sub.done) })
}

// deliver hands one frame to the subscriber, blocking until it is taken. It reports false only when
// the subscription ended while waiting -- never when the frame was dropped, because it never drops
// one: the caller then routes the frame to the legacy transport instead.
func (sub *embedOutboundSub) deliver(ctx context.Context, data []byte) (bool, error) {
	select {
	case sub.ch <- data:
		return true, nil
	case <-sub.done:
		return false, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

type embedEntry struct {
	reg        domain.EmbedRegistration
	tunnelOpen map[string]bool
	limiter    domain.RateLimiter
	wsConns    map[string]*embedWSConn
	active     bool
}

// embedWSSendQueueDepth bounds the frames a tunnel may hold between the sink and the wire.
//
// D10: "accepted downstream" means ws.WriteMessage returned, not that this queue took the frame.
// RouteTunnelFrameFromPlugin returns as soon as the queue accepts, so the plugin's credit window
// reopens on enqueue -- which means any depth this queue has is memory the credit window does not
// account for and the plugin's Job Object does not bound. At the old depth of 256 that was
// 256 x 64 KiB = 16 MiB per tunnel behind a 512 KiB window: 33x, invisible only because the pump
// never ran.
//
// Pinning the depth to the embed-stream credit window makes the distinction stop mattering: the
// queue can no longer hide more than a window, so the window is once again the only thing holding
// frames and per-tunnel host cost is the 8 x 64 KiB = 512 KiB ADR-011 promises. The queue still
// exists to decouple the sink from a single in-flight ws.WriteMessage; it is not a buffer.
//
// This must NOT drop (7.3). The enqueue below stays a non-blocking select whose full case is a
// classified ws-buffer-full refusal, which the embed backend's pump waits on (B4/D4). Making a
// full queue drop instead would lose a KeyEvent{down=0} and leave a modifier held on the remote
// machine with nothing in any log.
var embedWSSendQueueDepth = domainplugin.InitialCredit(domainplugin.PurposeEmbedStream)

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
		outbound:       make(map[string]map[string]*embedOutboundSub),
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

// SetBaseURL sets the absolute origin (e.g. "http://127.0.0.1:52731") prepended to embed UI and
// tunnel paths, so the browser resolves both from the real loopback listener that can serve the
// ws:// tunnel (see baseURL). Empty leaves the URLs root-relative.
func (s *EmbedTunnelService) SetBaseURL(u string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseURL = u
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
		return domain.SessionEmbedDescriptor{}, fmt.Errorf(
			"too many tunnels: %d requested, limit is %d per embed session",
			len(reg.TunnelIDs), domain.MaxTunnelsPerSession)
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
		// Open (ready to receive) on registration, not false. A registered-but-not-yet-attached
		// tunnel must classify a pre-attach frame as TunnelNotAttached (transient — the plugin, for a
		// server-speaks-first protocol like VNC, pushes its synthetic RFB banner before the iframe's
		// WebSocket attaches) so the host retries until attach. false made it TunnelClosed (terminal),
		// tearing the embed channel down and dropping the banner. Only CloseTunnel sets it false. The
		// channel-bus embed path never calls the legacy session.tunnelOpen, so this is the only place
		// a channel-bus tunnel is ever marked ready.
		entry.tunnelOpen[id] = true
	}
	s.entries[token] = entry
	s.sessionToken[reg.SessionID] = token
	ready := s.onEmbedReady
	s.mu.Unlock()

	mainTunnel := reg.TunnelIDs[0]
	desc := domain.SessionEmbedDescriptor{
		SessionID: reg.SessionID,
		PluginID:  reg.PluginID,
		UIUrl:     s.baseURL + embedUIPath(token, reg.UIEntry),
		TunnelUrl: s.baseURL + embedTunnelPath(token, mainTunnel),
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
		UIUrl:     s.baseURL + embedUIPath(token, entry.reg.UIEntry),
		TunnelUrl: s.baseURL + embedTunnelPath(token, mainTunnel),
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
//
// Every refusal is classified (see EmbedFrameRefusedError): this service is the only thing that
// knows whether a refusal means "wait a moment", "wait for the user to come back" or "this frame
// can never be delivered", and the two sentinels it returns cannot carry that difference. The
// sentinels are wrapped, never replaced, so callers matching them with errors.Is — the legacy
// ADR-008 tunnel path and ipc.HostServer's JSON-RPC code mapping — are unaffected.
func (s *EmbedTunnelService) RouteTunnelFrameFromPlugin(_ context.Context, sessionID, tunnelID string, data []byte) error {
	if len(data) > domain.MaxTunnelFrameSize {
		return newEmbedRefusal(EmbedRefusedFrameTooLarge, domainplugin.ErrRateLimited)
	}
	s.mu.RLock()
	entry, err := s.entryForSessionLocked(sessionID)
	if err != nil {
		s.mu.RUnlock()
		return newEmbedRefusal(EmbedRefusedSessionGone, err)
	}
	if !entry.active {
		s.mu.RUnlock()
		return newEmbedRefusal(EmbedRefusedTabInactive, domainplugin.ErrTerminalBackpressure)
	}
	if !entry.limiter.AllowN(len(data)) {
		s.mu.RUnlock()
		return newEmbedRefusal(EmbedRefusedRateLimited, domainplugin.ErrRateLimited)
	}
	conn := entry.wsConns[tunnelID]
	open, registered := entry.tunnelOpen[tunnelID]
	s.mu.RUnlock()
	if conn == nil {
		// An absent conn is three different states, and returning nil for all of them said
		// "delivered" about a frame nobody received. tunnelOpen tells them apart:
		switch {
		case !registered:
			// The plugin is addressing a tunnel this session never registered. Terminal, and its
			// own bug: no amount of waiting makes an id appear that was never minted.
			return newEmbedRefusal(EmbedRefusedTunnelUnknown, domainplugin.ErrHandleNotFound)
		case open:
			// Registered and open, but the iframe has not called AttachWebSocket yet. Transient
			// and legitimate: a plugin may push an RFB handshake before the browser is ready.
			return newEmbedRefusal(EmbedRefusedTunnelNotAttached, domainplugin.ErrTerminalBackpressure)
		default:
			// CloseTunnel removed the conn and left the entry alive, so nothing else ever reports
			// this: unlike RevokeBySession, which deletes the whole entry and surfaces as
			// session-revoked, a closed tunnel is invisible to every other check here.
			return newEmbedRefusal(EmbedRefusedTunnelClosed, domainplugin.ErrHandleNotFound)
		}
	}
	select {
	case conn.send <- data:
		pluginID, _ := s.PluginIDForSession(sessionID)
		slog.Debug("embed: plugin->browser frame queued to WS", "pluginId", pluginID, "sessionId", sessionID, "tunnelId", tunnelID, "bytes", len(data))
		return nil
	default:
		return newEmbedRefusal(EmbedRefusedWSBufferFull, domainplugin.ErrTerminalBackpressure)
	}
}

// RouteTunnelFrameToPlugin forwards browser WebSocket bytes to the plugin.
//
// This is D1's one predicate, and it lives here because this is the one place both transports are
// reachable and the only place the broker's pumpWSToPlugin calls: for a given (sessionID,
// tunnelID), input goes to exactly one transport. An open embed-stream channel REPLACES
// session.tunnelData for that tunnel -- it does not supplement it -- because a plugin subscribed to
// both would receive every input event twice, once as a binary frame and once as base64 JSON.
// Deciding this at two call sites is how that drifts back apart, so there is only ever this one.
func (s *EmbedTunnelService) RouteTunnelFrameToPlugin(ctx context.Context, sessionID, tunnelID string, data []byte) error {
	s.mu.RLock()
	entry, err := s.entryForSessionLocked(sessionID)
	notify := s.notifyPlugin
	sub := s.outboundSubLocked(sessionID, tunnelID)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	pluginID, _ := s.PluginIDForSession(sessionID)
	slog.Debug("embed: browser->plugin frame received from WS", "pluginId", pluginID, "sessionId", sessionID, "tunnelId", tunnelID, "bytes", len(data), "hasSub", sub != nil)
	if sub != nil {
		// Blocking, and deliberately outside the lock: the subscription carries the channel bus's
		// credit-based backpressure back to the browser's WebSocket read loop instead of queueing
		// input in the host. The tab-active check below is NOT applied here -- input is state
		// transitions, and dropping a KeyEvent{down=0} because a tab blurred leaves a modifier held
		// on the remote machine (7.3, D7's closing note).
		delivered, err := sub.deliver(ctx, data)
		if err != nil {
			return err
		}
		if delivered {
			return nil
		}
		// The channel unsubscribed while this frame was in hand. Fall through to the legacy
		// transport rather than drop the event: still one predicate, still one place.
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
		send: make(chan []byte, embedWSSendQueueDepth),
		done: make(chan struct{}),
	}
	entry.wsConns[tunnelID] = conn
	entry.tunnelOpen[tunnelID] = true
	entry.active = s.sessionActive[entry.reg.SessionID]
	return conn, entry.reg, nil
}

// SubscribeOutbound registers an embed-stream channel as the consumer of a tunnel's
// browser->plugin input, and is the plugin-side counterpart of AttachWebSocket: one registers the
// browser end of a tunnel, this registers the plugin end.
//
// The returned channel is unbuffered -- see embedOutboundSub for why that is the contract and not
// an oversight. The returned func unsubscribes; it is idempotent and safe to call concurrently with
// delivery, and after it returns the tunnel falls back to the legacy session.tunnelData
// notification (D1).
//
// Like AttachWebSocket, the last subscriber for a (sessionID, tunnelID) wins: a re-opened channel
// replaces a stale one rather than splitting the input stream between two consumers.
func (s *EmbedTunnelService) SubscribeOutbound(sessionID, tunnelID string) (<-chan []byte, func()) {
	if s == nil || sessionID == "" || tunnelID == "" {
		return nil, func() {}
	}
	sub := &embedOutboundSub{
		ch:   make(chan []byte),
		done: make(chan struct{}),
	}

	s.mu.Lock()
	byTunnel, ok := s.outbound[sessionID]
	if !ok {
		byTunnel = make(map[string]*embedOutboundSub)
		s.outbound[sessionID] = byTunnel
	}
	prev := byTunnel[tunnelID]
	byTunnel[tunnelID] = sub
	// The channel-bus embed path never calls session.tunnelOpen (that is the legacy ADR-008 path):
	// wiring the embed-stream channel — which is what triggers this subscription — is itself the
	// plugin declaring the tunnel ready. Mark it open so a plugin's pre-attach RFB banner (a VNC
	// server speaks first, and the plugin pushes the synthetic handshake before the iframe's
	// WebSocket attaches) is classified as "browser not attached yet" (transient, retried until the
	// WS attaches) rather than "tunnel closed" (terminal), which would tear the channel down and
	// drop the banner before noVNC ever receives it.
	if entry, entryErr := s.entryForSessionLocked(sessionID); entryErr == nil {
		if _, registered := entry.tunnelOpen[tunnelID]; registered {
			entry.tunnelOpen[tunnelID] = true
		}
	}
	s.mu.Unlock()

	if prev != nil {
		prev.stop()
	}

	return sub.ch, func() {
		s.mu.Lock()
		// Only this subscription's own registration is removed: a later SubscribeOutbound for the
		// same tunnel already replaced it, and must not be unsubscribed by its predecessor's func.
		if byTunnel, ok := s.outbound[sessionID]; ok && byTunnel[tunnelID] == sub {
			delete(byTunnel, tunnelID)
			if len(byTunnel) == 0 {
				delete(s.outbound, sessionID)
			}
		}
		s.mu.Unlock()
		sub.stop()
	}
}

// outboundSubLocked reports the channel subscriber for a tunnel, if any. Callers hold s.mu.
func (s *EmbedTunnelService) outboundSubLocked(sessionID, tunnelID string) *embedOutboundSub {
	byTunnel, ok := s.outbound[sessionID]
	if !ok {
		return nil
	}
	return byTunnel[tunnelID]
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
