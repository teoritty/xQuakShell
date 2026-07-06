package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

// PluginTunnelStarter ensures a tunnel-provider plugin process is running.
type PluginTunnelStarter interface {
	EnsureRunning(ctx context.Context, pluginID string) error
}

// DynamicForwardCoordinator manages dynamic (-D) forward rules bound to SSH sessions.
type DynamicForwardCoordinator struct {
	mu       sync.Mutex
	sessions map[string]*sessionDynamicForward
	rules    map[string]string // ruleID -> sessionID
	notify   PluginTunnelNotifier
	starter  PluginTunnelStarter
}

type sessionDynamicForward struct {
	sessionID string
	service   *TunnelDynamicService
	listeners map[string]net.Listener
	cancel    context.CancelFunc
}

// NewDynamicForwardCoordinator creates a coordinator for plugin-mediated dynamic forwards.
func NewDynamicForwardCoordinator(notify PluginTunnelNotifier, starter PluginTunnelStarter) *DynamicForwardCoordinator {
	return &DynamicForwardCoordinator{
		sessions: make(map[string]*sessionDynamicForward),
		rules:    make(map[string]string),
		notify:   notify,
		starter:  starter,
	}
}

// SetNotifier wires host→plugin tunnel notifications.
func (c *DynamicForwardCoordinator) SetNotifier(notify PluginTunnelNotifier) {
	if c != nil {
		c.notify = notify
	}
}

// SetStarter wires lazy plugin process startup.
func (c *DynamicForwardCoordinator) SetStarter(starter PluginTunnelStarter) {
	if c != nil {
		c.starter = starter
	}
}

// StartSession activates enabled dynamic forward rules for a live SSH session.
func (c *DynamicForwardCoordinator) StartSession(parentCtx context.Context, sessionID string, dialer domain.TunnelChannelDialer, rules []domain.ForwardRule) {
	if c == nil || dialer == nil {
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	svc := NewTunnelDynamicService(dialer, c.notify)
	sf := &sessionDynamicForward{
		sessionID: sessionID,
		service:   svc,
		listeners: make(map[string]net.Listener),
		cancel:    cancel,
	}
	c.mu.Lock()
	c.sessions[sessionID] = sf
	c.mu.Unlock()

	for _, rule := range rules {
		if !rule.Enabled || rule.Kind != domain.ForwardRuleDynamic {
			continue
		}
		if err := rule.Validate(); err != nil {
			continue
		}
		c.mu.Lock()
		c.rules[rule.ID] = sessionID
		c.mu.Unlock()
		if err := c.startDynamicRule(ctx, sf, rule); err != nil {
			c.mu.Lock()
			delete(c.rules, rule.ID)
			c.mu.Unlock()
		}
	}
}

func (c *DynamicForwardCoordinator) startDynamicRule(ctx context.Context, sf *sessionDynamicForward, rule domain.ForwardRule) error {
	if c.starter != nil {
		if err := c.starter.EnsureRunning(ctx, rule.PluginID); err != nil {
			return err
		}
	}
	bindAddr := rule.BindAddress
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindAddr, strconv.Itoa(rule.BindPort)))
	if err != nil {
		return fmt.Errorf("listen dynamic forward %s: %w", rule.ID, err)
	}
	sf.listeners[rule.ID] = ln
	safego.GoNamed("dynamicForward.accept", func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			safego.Go(func() { c.handleLocalAccept(ctx, sf, rule, conn) })
		}
	})
	return nil
}

func (c *DynamicForwardCoordinator) handleLocalAccept(ctx context.Context, sf *sessionDynamicForward, rule domain.ForwardRule, conn net.Conn) {
	localConnID, err := newTunnelHandleID()
	if err != nil {
		conn.Close()
		return
	}
	if err := sf.service.RegisterLocal(ctx, rule.PluginID, rule.ID, localConnID, conn); err != nil {
		conn.Close()
		return
	}
	safego.Go(func() {
		timer := time.NewTimer(domainplugin.PreBindTunnelTimeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			_ = sf.service.CloseLocal(localConnID)
		case <-timer.C:
			_ = sf.service.CloseLocal(localConnID)
		}
	})
}

// StopSession tears down listeners and pre-bind state for a session.
func (c *DynamicForwardCoordinator) StopSession(sessionID string) {
	c.mu.Lock()
	sf, ok := c.sessions[sessionID]
	if ok {
		delete(c.sessions, sessionID)
		for ruleID, sid := range c.rules {
			if sid == sessionID {
				delete(c.rules, ruleID)
			}
		}
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	sf.cancel()
	for _, ln := range sf.listeners {
		ln.Close()
	}
}

func (c *DynamicForwardCoordinator) sessionForRule(ruleID string) (*sessionDynamicForward, error) {
	c.mu.Lock()
	sessionID, ok := c.rules[ruleID]
	sf := c.sessions[sessionID]
	c.mu.Unlock()
	if !ok || sf == nil {
		return nil, domainplugin.ErrTunnelNotFound
	}
	return sf, nil
}

// TunnelDial implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelDial(ctx context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		RuleID     string `json:"ruleId"`
		TargetHost string `json:"targetHost"`
		TargetPort int    `json:"targetPort"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	sf, err := c.sessionForRule(req.RuleID)
	if err != nil {
		return nil, err
	}
	tunnelID, err := newTunnelHandleID()
	if err != nil {
		return nil, err
	}
	if err := sf.service.Dial(ctx, tunnelID, req.TargetHost, req.TargetPort); err != nil {
		return nil, err
	}
	_ = pluginID
	return json.Marshal(map[string]string{"tunnelId": tunnelID})
}

// TunnelClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelClose(_ context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceWithChannel(req.TunnelID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseChannel(req.TunnelID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalWrite implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalWrite(_ context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
		DataBase64  string `json:"dataBase64"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return nil, err
	}
	svc, err := c.serviceWithLocal(req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.WriteLocal(req.LocalConnID, data); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalClose(_ context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceWithLocal(req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseLocal(req.LocalConnID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelBind implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelBind(_ context.Context, _ string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
		TunnelID    string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceWithLocal(req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if !svc.HasChannel(req.TunnelID) {
		return nil, domainplugin.ErrTunnelNotFound
	}
	if err := svc.Bind(req.LocalConnID, req.TunnelID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

func (c *DynamicForwardCoordinator) serviceWithLocal(localConnID string) (*TunnelDynamicService, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sf := range c.sessions {
		if sf.service != nil && sf.service.HasLocal(localConnID) {
			return sf.service, nil
		}
	}
	return nil, domainplugin.ErrTunnelNotFound
}

func (c *DynamicForwardCoordinator) serviceWithChannel(tunnelID string) (*TunnelDynamicService, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sf := range c.sessions {
		if sf.service != nil && sf.service.HasChannel(tunnelID) {
			return sf.service, nil
		}
	}
	return nil, domainplugin.ErrTunnelNotFound
}

var _ domainplugin.TunnelInboundPort = (*DynamicForwardCoordinator)(nil)

func newTunnelHandleID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
