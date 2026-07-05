package embed

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/plugin/assets"
	"ssh-client/internal/pkg/pathsafe"
	"ssh-client/internal/pkg/safego"

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
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.ServeFile(w, r, absTarget)
	_ = token
}

func (h *BrokerHandler) serveTunnel(w http.ResponseWriter, r *http.Request, token, tunnelID string, reg domain.EmbedRegistration) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	conn, regData, err := h.tunnels.AttachWebSocket(token, tunnelID)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
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
			return
		}
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
				return
			}
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
