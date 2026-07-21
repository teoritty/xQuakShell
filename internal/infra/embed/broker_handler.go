package embed

import (
	"log/slog"
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
	failMu   sync.Mutex
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
	// Tagged with a pluginId once known so loghub routes it to the plugin's log stream (the debug
	// window shows plugin:* sources, not bare core). Before Lookup the id is unknown, so this
	// first line carries a synthetic marker id purely so it is visible at all.
	slog.Debug("embed broker: request", "pluginId", "com.xquakshell.vnc", "method", r.Method, "path", path, "upgrade", r.Header.Get("Upgrade"))
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
	reg, err := h.tunnels.Lookup(token)
	if err != nil {
		slog.Debug("embed broker: token lookup failed", "pluginId", "com.xquakshell.vnc", "path", path, "err", err.Error())
		h.recordFailedLookup(r)
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
	safego.GoNamed("embed.pumpWS", func() { h.pumpWSToPlugin(r, regData.SessionID, tunnelID, ws, conn) })
	h.pumpPluginToWS(ws, conn)
}

func (h *BrokerHandler) pumpWSToPlugin(r *http.Request, sessionID, tunnelID string, ws *websocket.Conn, conn domain.EmbedTunnelStream) {
	defer ws.Close()
	for {
		select {
		case <-conn.Done():
			return
		default:
		}
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			slog.Debug("embed broker: WS read ended", "pluginId", "com.xquakshell.vnc", "tunnelId", tunnelID, "err", err.Error())
			return
		}
		slog.Debug("embed broker: WS read from browser", "pluginId", "com.xquakshell.vnc", "tunnelId", tunnelID, "msgType", msgType, "bytes", len(data))
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if len(data) > domain.MaxTunnelFrameSize {
			continue
		}
		_ = h.tunnels.RouteTunnelFrameToPlugin(r.Context(), sessionID, tunnelID, data)
	}
}

func (h *BrokerHandler) pumpPluginToWS(ws *websocket.Conn, conn domain.EmbedTunnelStream) {
	for {
		select {
		case data, ok := <-conn.Send():
			if !ok {
				return
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
				slog.Debug("embed broker: WS write to browser failed", "pluginId", "com.xquakshell.vnc", "err", err.Error())
				return
			}
			slog.Debug("embed broker: WS wrote to browser", "pluginId", "com.xquakshell.vnc", "bytes", len(data))
		case <-conn.Done():
			return
		}
	}
}

func (h *BrokerHandler) recordFailedLookup(r *http.Request) {
	h.failMu.Lock()
	defer h.failMu.Unlock()
	key := r.RemoteAddr
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
	h.failures[key] = filtered
	if len(filtered) >= maxFailedLookups {
		http.Error(nil, "too many requests", http.StatusTooManyRequests)
	}
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
