package guacamole

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionSettings holds guacd connection parameters for a single RDP session.
type ConnectionSettings struct {
	Protocol   string
	Hostname   string
	Port       int
	Username   string
	Password   string
	Domain     string
	Width      int
	Height     int
	DPI        int
	Security   string
	IgnoreCert bool
	Audio      []string
	Video      []string
	Image      []string
	Timezone   string
}

type gwSession struct {
	settings ConnectionSettings
	created  time.Time
}

// Gateway is a WebSocket server that proxies Guacamole protocol between
// guacamole-common-js (running in the frontend) and a local guacd process.
type Gateway struct {
	mu       sync.RWMutex
	sessions map[string]*gwSession

	guacdHost string
	guacdPort int

	listener net.Listener
	server   *http.Server
	port     int

	upgrader websocket.Upgrader
}

// NewGateway creates a new Guacamole gateway.
func NewGateway() *Gateway {
	return &Gateway{
		sessions: make(map[string]*gwSession),
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  32 * 1024,
			WriteBufferSize: 32 * 1024,
		},
	}
}

// Start starts the gateway WebSocket server on a random port.
// guacdHost/guacdPort specify where the local guacd is listening.
func (gw *Gateway) Start(guacdHost string, guacdPort int) (int, error) {
	gw.guacdHost = guacdHost
	gw.guacdPort = guacdPort

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("gateway listen: %w", err)
	}
	gw.listener = listener
	gw.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", gw.handleWebSocket)

	gw.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  0,
		WriteTimeout: 0,
	}

	go func() {
		if err := gw.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[gateway] server error: %v", err)
		}
	}()

	log.Printf("[gateway] started on 127.0.0.1:%d → guacd %s:%d", gw.port, guacdHost, guacdPort)
	return gw.port, nil
}

// Stop shuts down the gateway server.
func (gw *Gateway) Stop() {
	if gw.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = gw.server.Shutdown(ctx)
	}
}

// Port returns the port the gateway is listening on.
func (gw *Gateway) Port() int {
	return gw.port
}

// RegisterSession stores connection settings for a pending session.
func (gw *Gateway) RegisterSession(sessionID string, settings ConnectionSettings) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.sessions[sessionID] = &gwSession{
		settings: settings,
		created:  time.Now(),
	}
}

// UnregisterSession removes a session from the gateway.
func (gw *Gateway) UnregisterSession(sessionID string) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	delete(gw.sessions, sessionID)
}

func (gw *Gateway) getSession(sessionID string) (*gwSession, bool) {
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	s, ok := gw.sessions[sessionID]
	return s, ok
}

func (gw *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session parameter", http.StatusBadRequest)
		return
	}

	sess, ok := gw.getSession(sessionID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}

	ws, err := gw.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[gateway] session %s: websocket upgrade failed: %v", sessionID, err)
		return
	}
	defer ws.Close()

	log.Printf("[gateway] session %s: websocket connected from %s", sessionID, r.RemoteAddr)

	guacdAddr := fmt.Sprintf("%s:%d", gw.guacdHost, gw.guacdPort)
	guacdConn, err := net.DialTimeout("tcp", guacdAddr, 10*time.Second)
	if err != nil {
		log.Printf("[gateway] session %s: failed to connect to guacd at %s: %v", sessionID, guacdAddr, err)
		sendWSError(ws, "Failed to connect to guacd: "+err.Error())
		return
	}
	defer guacdConn.Close()

	log.Printf("[gateway] session %s: connected to guacd", sessionID)

	connectionID, err := gw.performGuacdHandshake(guacdConn, sess.settings)
	if err != nil {
		log.Printf("[gateway] session %s: guacd handshake failed: %v", sessionID, err)
		sendWSError(ws, "Guacd handshake failed: "+err.Error())
		return
	}

	log.Printf("[gateway] session %s: handshake complete, guacd connectionID=%s", sessionID, connectionID)

	internalInstruction := FormatInstruction("", connectionID)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(internalInstruction)); err != nil {
		log.Printf("[gateway] session %s: failed to send connection ID to client: %v", sessionID, err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gw.proxyGuacdToWS(ctx, cancel, sessionID, guacdConn, ws)
	go gw.proxyWSToGuacd(ctx, cancel, sessionID, ws, guacdConn)

	<-ctx.Done()
	log.Printf("[gateway] session %s: proxy ended", sessionID)
}

func (gw *Gateway) proxyGuacdToWS(ctx context.Context, cancel context.CancelFunc, sessionID string, guacd net.Conn, ws *websocket.Conn) {
	defer cancel()
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := guacd.Read(buf)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[gateway] session %s: guacd read error: %v", sessionID, err)
			}
			return
		}
		if err := ws.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
			if ctx.Err() == nil {
				log.Printf("[gateway] session %s: ws write error: %v", sessionID, err)
			}
			return
		}
	}
}

func (gw *Gateway) proxyWSToGuacd(ctx context.Context, cancel context.CancelFunc, sessionID string, ws *websocket.Conn, guacd net.Conn) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[gateway] session %s: ws read error: %v", sessionID, err)
			}
			return
		}
		if _, err := guacd.Write(msg); err != nil {
			if ctx.Err() == nil {
				log.Printf("[gateway] session %s: guacd write error: %v", sessionID, err)
			}
			return
		}
	}
}

func (gw *Gateway) performGuacdHandshake(conn net.Conn, settings ConnectionSettings) (string, error) {
	protocol := settings.Protocol
	if protocol == "" {
		protocol = "rdp"
	}

	selectInstr := FormatInstruction("select", protocol)
	if _, err := conn.Write([]byte(selectInstr)); err != nil {
		return "", fmt.Errorf("send select: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	parser := &InstructionParser{}
	buf := make([]byte, 16*1024)

	var argNames []string
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return "", fmt.Errorf("read args from guacd: %w", err)
		}
		parser.Feed(string(buf[:n]))

		opcode, args, err := parser.Next()
		if err != nil {
			return "", fmt.Errorf("parse args: %w", err)
		}
		if opcode == "args" {
			argNames = args
			break
		}
		if opcode == "error" {
			errMsg := "unknown error"
			if len(args) > 0 {
				errMsg = args[0]
			}
			return "", fmt.Errorf("guacd rejected select: %s", errMsg)
		}
		if opcode != "" {
			return "", fmt.Errorf("expected 'args', got %q", opcode)
		}
	}

	settingsMap := buildSettingsMap(settings)

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	width := strconv.Itoa(settings.Width)
	height := strconv.Itoa(settings.Height)
	dpi := "96"
	if settings.DPI > 0 {
		dpi = strconv.Itoa(settings.DPI)
	}

	for _, instr := range []string{
		FormatInstruction("size", width, height, dpi),
		FormatInstruction(append([]string{"audio"}, settings.Audio...)...),
		FormatInstruction(append([]string{"video"}, settings.Video...)...),
		FormatInstruction(append([]string{"image"}, settings.Image...)...),
	} {
		if _, err := conn.Write([]byte(instr)); err != nil {
			return "", fmt.Errorf("send handshake instruction: %w", err)
		}
	}

	if settings.Timezone != "" {
		if _, err := conn.Write([]byte(FormatInstruction("timezone", settings.Timezone))); err != nil {
			return "", fmt.Errorf("send timezone: %w", err)
		}
	} else {
		if _, err := conn.Write([]byte(FormatInstruction("timezone"))); err != nil {
			return "", fmt.Errorf("send timezone: %w", err)
		}
	}

	connectArgs := make([]string, 0, len(argNames)+1)
	connectArgs = append(connectArgs, "connect")
	for _, argName := range argNames {
		if strings.HasPrefix(argName, "VERSION_") {
			connectArgs = append(connectArgs, argName)
		} else if val, ok := settingsMap[argName]; ok {
			connectArgs = append(connectArgs, val)
		} else {
			connectArgs = append(connectArgs, "")
		}
	}

	if _, err := conn.Write([]byte(FormatInstruction(connectArgs...))); err != nil {
		return "", fmt.Errorf("send connect: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	var connectionID string
	for connectionID == "" {
		n, err := conn.Read(buf)
		if err != nil {
			return "", fmt.Errorf("read ready from guacd: %w", err)
		}
		parser.Feed(string(buf[:n]))

		for {
			opcode, args, err := parser.Next()
			if err != nil {
				return "", fmt.Errorf("parse ready: %w", err)
			}
			if opcode == "" {
				break
			}
			if opcode == "ready" && len(args) > 0 {
				connectionID = args[0]
				break
			}
			if opcode == "error" {
				errMsg := "unknown"
				if len(args) > 0 {
					errMsg = args[0]
				}
				return "", fmt.Errorf("guacd error during connect: %s", errMsg)
			}
		}
	}

	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})

	return connectionID, nil
}

func buildSettingsMap(s ConnectionSettings) map[string]string {
	m := map[string]string{
		"hostname": s.Hostname,
		"port":     strconv.Itoa(s.Port),
	}
	if s.Username != "" {
		m["username"] = s.Username
	}
	if s.Password != "" {
		m["password"] = s.Password
	}
	if s.Domain != "" {
		m["domain"] = s.Domain
	}
	if s.Security != "" {
		m["security"] = s.Security
	}
	if s.IgnoreCert {
		m["ignore-cert"] = "true"
	}
	if s.Width > 0 {
		m["width"] = strconv.Itoa(s.Width)
	}
	if s.Height > 0 {
		m["height"] = strconv.Itoa(s.Height)
	}
	if s.DPI > 0 {
		m["dpi"] = strconv.Itoa(s.DPI)
	}
	return m
}

func sendWSError(ws *websocket.Conn, msg string) {
	errInstr := FormatInstruction("error", msg, "519")
	_ = ws.WriteMessage(websocket.TextMessage, []byte(errInstr))
}
