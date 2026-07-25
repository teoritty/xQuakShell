package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/plugin/assets"
	"xquakshell/internal/pkg/pathsafe"
	"xquakshell/internal/pkg/safego"

	"github.com/gorilla/websocket"
)

const (
	embedPrefix        = "/embed/s/"
	maxFailedLookups   = 20
	failedLookupWindow = time.Minute
)

var wsUpgrader = websocket.Upgrader{
	Subprotocols:    []string{"xqs.embed.v1"},
	ReadBufferSize:  domain.MaxTunnelFrameSize,
	WriteBufferSize: domain.MaxTunnelFrameSize,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// UIRootResolver returns the ui/ directory for a plugin ID.
type UIRootResolver func(pluginID string) (string, error)

// BrokerHandler serves embed UI assets and WebSocket tunnels.
type BrokerHandler struct {
	tunnels  domain.EmbedTunnelPort
	resolve  UIRootResolver
	failMu   sync.RWMutex
	failures map[string][]time.Time
}

// NewBrokerHandler creates an embed broker HTTP handler.
func NewBrokerHandler(tunnels domain.EmbedTunnelPort, resolve UIRootResolver) *BrokerHandler {
	return &BrokerHandler{
		tunnels:  tunnels,
		resolve:  resolve,
		failures: make(map[string][]time.Time),
	}
}

// ServeHTTP implements http.Handler for /embed/s/{token}/… routes.
func (h *BrokerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.tunnels == nil {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, embedPrefix) {
		http.NotFound(w, r)
		return
	}
	rest := strings.TrimPrefix(path, embedPrefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	token := parts[0]
	// Tagged with a pluginId once known so loghub routes it to the plugin's log stream (the debug
	// window shows plugin:* sources, not bare core). Before Lookup the id is unknown, so this
	// first line carries an explicit "unknown" placeholder rather than a guessed plugin id. The
	// full path is never logged: it contains the embed token (parts[0]), the only thing gating
	// access to the tunnel, so only a non-reversible tag derived from it is logged instead.
	slog.Debug("embed broker: request", "pluginId", "unknown", "method", r.Method, "route", parts[1], "token", tokenTag(token), "upgrade", r.Header.Get("Upgrade"))
	if h.overBudget(r) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	reg, err := h.tunnels.Lookup(token)
	if err != nil {
		slog.Debug("embed broker: token lookup failed", "pluginId", "unknown", "token", tokenTag(token), "err", err.Error())
		if h.recordFailedLookup(r) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch parts[1] {
	case "ui":
		h.serveUI(w, r, token, reg, parts)
	case "tunnel":
		if len(parts) < 3 || parts[2] == "" {
			http.NotFound(w, r)
			return
		}
		h.serveTunnel(w, r, token, parts[2], reg)
	default:
		http.NotFound(w, r)
	}
}

func (h *BrokerHandler) serveUI(w http.ResponseWriter, r *http.Request, token string, reg domain.EmbedRegistration, parts []string) {
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(parts[2], "/")
	if rel == "" {
		rel = "index.html"
	}
	uiEntry := reg.UIEntry
	base := filepath.Base(uiEntry)
	if rel == "index.html" {
		rel = base
	}
	relPath, err := assets.ResolveUIRelPath(rel)
	if err != nil || !assets.IsAllowedAssetName(filepath.Base(relPath)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	uiRoot, err := h.resolve(reg.PluginID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	absRoot, err := filepath.Abs(uiRoot)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	target := filepath.Join(absRoot, relPath)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if !pathsafe.UnderRoot(absRoot, absTarget) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// The embed UI is DESIGNED to be rendered inside the app's own iframe, so it must NOT carry
	// X-Frame-Options: DENY (which blocks all framing — the browser then refuses to render the
	// document in the frame, leaving a blank panel and never running its scripts). The document is
	// served from a loopback origin (127.0.0.1:<port>) and framed by the wails.localhost app — a
	// cross-origin pair — so frame-ancestors 'self' would wrongly block it; access is gated by the
	// unguessable per-session embed token instead. connect-src 'self' covers the same-origin ws://
	// tunnel on that loopback host.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Serve the content directly rather than via http.ServeFile: ServeFile issues a 301 redirect
	// for any path ending in /index.html (-> "./"), which moves the iframe document to ".../ui/"
	// and breaks the WebView2 asset interception of its module-script subresource (./boot.js),
	// leaving the embed a blank frame. ServeContent has no such redirect.
	f, openErr := os.Open(absTarget)
	if openErr != nil {
		slog.Debug("embed broker: UI asset not found", "pluginId", reg.PluginID, "rel", rel, "target", absTarget, "err", openErr.Error())
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, statErr := f.Stat()
	if statErr != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	slog.Debug("embed broker: UI asset served", "pluginId", reg.PluginID, "rel", rel)
	http.ServeContent(w, r, filepath.Base(absTarget), info.ModTime(), f)
	_ = token
}

func (h *BrokerHandler) serveTunnel(w http.ResponseWriter, r *http.Request, token, tunnelID string, reg domain.EmbedRegistration) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	conn, regData, err := h.tunnels.AttachWebSocket(token, tunnelID)
	if err != nil {
		slog.Debug("embed broker: tunnel attach failed", "pluginId", reg.PluginID, "tunnelId", tunnelID, "err", err.Error())
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Debug("embed broker: websocket upgrade failed", "pluginId", reg.PluginID, "tunnelId", tunnelID, "err", err.Error())
		return
	}
	defer ws.Close()
	slog.Debug("embed broker: tunnel websocket attached", "pluginId", reg.PluginID, "tunnelId", tunnelID, "sessionId", regData.SessionID)
	safego.GoNamed("embed.pumpWS", func() { h.pumpWSToPlugin(r, reg.PluginID, regData.SessionID, tunnelID, ws, conn) })
	h.pumpPluginToWS(reg.PluginID, ws, conn)
}

func (h *BrokerHandler) pumpWSToPlugin(r *http.Request, pluginID, sessionID, tunnelID string, ws *websocket.Conn, conn domain.EmbedTunnelStream) {
	defer ws.Close()
	for {
		select {
		case <-conn.Done():
			return
		default:
		}
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			slog.Debug("embed broker: WS read ended", "pluginId", pluginID, "tunnelId", tunnelID, "err", err.Error())
			return
		}
		slog.Debug("embed broker: WS read from browser", "pluginId", pluginID, "tunnelId", tunnelID, "msgType", msgType, "bytes", len(data))
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if len(data) > domain.MaxTunnelFrameSize {
			continue
		}
		_ = h.tunnels.RouteTunnelFrameToPlugin(r.Context(), sessionID, tunnelID, data)
	}
}

func (h *BrokerHandler) pumpPluginToWS(pluginID string, ws *websocket.Conn, conn domain.EmbedTunnelStream) {
	for {
		select {
		case data, ok := <-conn.Send():
			if !ok {
				return
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				slog.Debug("embed broker: WS write to browser failed", "pluginId", pluginID, "err", err.Error())
				return
			}
			slog.Debug("embed broker: WS wrote to browser", "pluginId", pluginID, "bytes", len(data))
		case <-conn.Done():
			return
		}
	}
}

// tokenTag returns a short, non-reversible tag for an embed token, safe to log. The raw token is
// a capability: anything that can read it can attach to the tunnel, and this log stream feeds the
// debug log window. A truncated SHA-256 keeps entries correlatable across log lines without
// carrying the secret — unlike a token prefix, which is part of the secret and shrinks the search
// space for anyone reading the log.
func tokenTag(token string) string {
	if token == "" {
		return "-"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
}

// failureKey reduces r.RemoteAddr (host:port) to just the host. The port is fresh for every TCP
// connection, so keying by RemoteAddr as-is would give every failed request its own bucket: the
// limiter would never trip, and the map would grow without bound.
func failureKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// overBudget is an advisory read: a concurrent request may slip through before recordFailedLookup
// commits. That is intentional — this is a brute-force brake, not an accounting boundary. It must
// NOT record anything, or a throttled caller would keep its own window alive forever and never
// escape it.
func (h *BrokerHandler) overBudget(r *http.Request) bool {
	h.failMu.RLock()
	defer h.failMu.RUnlock()
	window := h.failures[failureKey(r)]
	cutoff := time.Now().Add(-failedLookupWindow)
	count := 0
	for _, t := range window {
		if t.After(cutoff) {
			count++
		}
	}
	return count >= maxFailedLookups
}

// recordFailedLookup registers a failed token lookup from r's remote address and reports whether
// the caller has exceeded the failure budget. It never writes a response: only ServeHTTP owns the
// ResponseWriter.
func (h *BrokerHandler) recordFailedLookup(r *http.Request) (throttled bool) {
	h.failMu.Lock()
	defer h.failMu.Unlock()
	key := failureKey(r)
	now := time.Now()
	window := h.failures[key]
	cutoff := now.Add(-failedLookupWindow)
	filtered := window[:0]
	for _, t := range window {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	filtered = append(filtered, now)
	if len(filtered) == 0 {
		delete(h.failures, key)
	} else {
		h.failures[key] = filtered
	}
	return len(filtered) >= maxFailedLookups
}

// CompositeHandler routes /plugin/* and /embed/* to dedicated handlers.
type CompositeHandler struct {
	PluginAssets http.Handler
	EmbedBroker  http.Handler
}

// NewCompositeHandler creates a composite asset server handler.
func NewCompositeHandler(pluginAssets, embedBroker http.Handler) *CompositeHandler {
	return &CompositeHandler{PluginAssets: pluginAssets, EmbedBroker: embedBroker}
}

// ServeHTTP dispatches by URL prefix.
func (c *CompositeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/embed/"):
		if c.EmbedBroker != nil {
			c.EmbedBroker.ServeHTTP(w, r)
			return
		}
	case strings.HasPrefix(r.URL.Path, "/plugin/"):
		if c.PluginAssets != nil {
			c.PluginAssets.ServeHTTP(w, r)
			return
		}
	}
	http.NotFound(w, r)
}
