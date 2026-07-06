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

// PluginTunnelGrantReader reports install-time tunnel provider consent.
type PluginTunnelGrantReader interface {
	IsTunnelProviderGranted(pluginID string) bool
}

type tunnelHandleOwner struct {
	sessionID string
	pluginID  string
}

type dynamicRuleEntry struct {
	sessionID string
	rule      domain.ForwardRule
}

// DynamicForwardCoordinator manages dynamic (-D) forward rules bound to SSH sessions.
type DynamicForwardCoordinator struct {
	mu             sync.Mutex
	sessions       map[string]*sessionDynamicForward
	rules          map[string]dynamicRuleEntry // ruleID -> binding
	localOwners    map[string]tunnelHandleOwner
	tunnelOwners   map[string]tunnelHandleOwner
	preBindDone    map[string]chan struct{}
	notify         PluginTunnelNotifier
	starter        PluginTunnelStarter
	tunnelGrant    PluginTunnelGrantReader
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
		sessions:     make(map[string]*sessionDynamicForward),
		rules:        make(map[string]dynamicRuleEntry),
		localOwners:  make(map[string]tunnelHandleOwner),
		tunnelOwners: make(map[string]tunnelHandleOwner),
		preBindDone:  make(map[string]chan struct{}),
		notify:       notify,
		starter:      starter,
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

// SetTunnelGrantReader wires install-time tunnel provider consent checks.
func (c *DynamicForwardCoordinator) SetTunnelGrantReader(reader PluginTunnelGrantReader) {
	if c != nil {
		c.tunnelGrant = reader
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
		c.rules[rule.ID] = dynamicRuleEntry{sessionID: sessionID, rule: rule}
		c.mu.Unlock()
		if err := c.startDynamicRule(ctx, sf, rule); err != nil {
			c.mu.Lock()
			delete(c.rules, rule.ID)
			c.mu.Unlock()
		}
	}
}

func (c *DynamicForwardCoordinator) startDynamicRule(ctx context.Context, sf *sessionDynamicForward, rule domain.ForwardRule) error {
	if c.tunnelGrant != nil && !c.tunnelGrant.IsTunnelProviderGranted(rule.PluginID) {
		return domainplugin.ErrCapabilityDenied
	}
	if c.starter != nil {
		if err := c.starter.EnsureRunning(ctx, rule.PluginID); err != nil {
			return err
		}
	}
	bindAddr := domain.EffectiveBindAddress(rule.BindAddress)
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

	bound := make(chan struct{})
	c.mu.Lock()
	c.localOwners[localConnID] = tunnelHandleOwner{sessionID: sf.sessionID, pluginID: rule.PluginID}
	c.preBindDone[localConnID] = bound
	c.mu.Unlock()

	safego.Go(func() {
		timer := time.NewTimer(domainplugin.PreBindTunnelTimeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			c.cleanupPreBindLocal(sf, localConnID)
		case <-timer.C:
			c.cleanupPreBindLocal(sf, localConnID)
		case <-bound:
		}
	})
}

func (c *DynamicForwardCoordinator) cleanupPreBindLocal(sf *sessionDynamicForward, localConnID string) {
	_ = sf.service.CloseLocal(localConnID)
	c.mu.Lock()
	delete(c.localOwners, localConnID)
	delete(c.preBindDone, localConnID)
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) markLocalBound(localConnID string) {
	c.mu.Lock()
	if done, ok := c.preBindDone[localConnID]; ok {
		close(done)
		delete(c.preBindDone, localConnID)
	}
	delete(c.localOwners, localConnID)
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) releaseTunnelOwner(tunnelID string) {
	c.mu.Lock()
	delete(c.tunnelOwners, tunnelID)
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) clearSessionOwners(sessionID string) {
	for id, owner := range c.localOwners {
		if owner.sessionID == sessionID {
			delete(c.localOwners, id)
			if done, ok := c.preBindDone[id]; ok {
				close(done)
				delete(c.preBindDone, id)
			}
		}
	}
	for id, owner := range c.tunnelOwners {
		if owner.sessionID == sessionID {
			delete(c.tunnelOwners, id)
		}
	}
}

// StopSession tears down listeners and pre-bind state for a session.
func (c *DynamicForwardCoordinator) StopSession(sessionID string) {
	c.mu.Lock()
	sf, ok := c.sessions[sessionID]
	if ok {
		delete(c.sessions, sessionID)
		for ruleID, entry := range c.rules {
			if entry.sessionID == sessionID {
				delete(c.rules, ruleID)
			}
		}
		c.clearSessionOwners(sessionID)
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

func (c *DynamicForwardCoordinator) sessionForRule(pluginID, ruleID string) (*sessionDynamicForward, error) {
	c.mu.Lock()
	entry, ok := c.rules[ruleID]
	sf := c.sessions[entry.sessionID]
	c.mu.Unlock()
	if !ok || entry.rule.PluginID != pluginID || sf == nil {
		return nil, domainplugin.ErrTunnelNotFound
	}
	return sf, nil
}

func (c *DynamicForwardCoordinator) serviceForLocal(pluginID, localConnID string) (*TunnelDynamicService, error) {
	c.mu.Lock()
	owner, ok := c.localOwners[localConnID]
	sf := c.sessions[owner.sessionID]
	c.mu.Unlock()
	if !ok || owner.pluginID != pluginID || sf == nil || sf.service == nil {
		return nil, domainplugin.ErrTunnelNotFound
	}
	return sf.service, nil
}

func (c *DynamicForwardCoordinator) serviceForChannel(pluginID, tunnelID string) (*TunnelDynamicService, error) {
	c.mu.Lock()
	owner, ok := c.tunnelOwners[tunnelID]
	sf := c.sessions[owner.sessionID]
	c.mu.Unlock()
	if !ok || owner.pluginID != pluginID || sf == nil || sf.service == nil {
		return nil, domainplugin.ErrTunnelNotFound
	}
	return sf.service, nil
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
	sf, err := c.sessionForRule(pluginID, req.RuleID)
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
	c.mu.Lock()
	c.tunnelOwners[tunnelID] = tunnelHandleOwner{sessionID: sf.sessionID, pluginID: pluginID}
	c.mu.Unlock()
	return json.Marshal(map[string]string{"tunnelId": tunnelID})
}

// TunnelClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelClose(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		TunnelID string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForChannel(pluginID, req.TunnelID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseChannel(req.TunnelID); err != nil {
		return nil, err
	}
	c.releaseTunnelOwner(req.TunnelID)
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalWrite implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalWrite(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
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
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.WriteLocal(req.LocalConnID, data); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelLocalClose implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelLocalClose(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if err := svc.CloseLocal(req.LocalConnID); err != nil {
		return nil, err
	}
	c.mu.Lock()
	delete(c.localOwners, req.LocalConnID)
	if done, ok := c.preBindDone[req.LocalConnID]; ok {
		close(done)
		delete(c.preBindDone, req.LocalConnID)
	}
	c.mu.Unlock()
	return json.Marshal(map[string]bool{"ok": true})
}

// TunnelBind implements domainplugin.TunnelInboundPort.
func (c *DynamicForwardCoordinator) TunnelBind(_ context.Context, pluginID string, params json.RawMessage) (json.RawMessage, error) {
	var req struct {
		LocalConnID string `json:"localConnId"`
		TunnelID    string `json:"tunnelId"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	svc, err := c.serviceForLocal(pluginID, req.LocalConnID)
	if err != nil {
		return nil, err
	}
	if _, err := c.serviceForChannel(pluginID, req.TunnelID); err != nil {
		return nil, err
	}
	if !svc.HasChannel(req.TunnelID) {
		return nil, domainplugin.ErrTunnelNotFound
	}
	if err := svc.Bind(req.LocalConnID, req.TunnelID); err != nil {
		return nil, err
	}
	c.markLocalBound(req.LocalConnID)
	c.releaseTunnelOwner(req.TunnelID)
	return json.Marshal(map[string]bool{"ok": true})
}

var _ domainplugin.TunnelInboundPort = (*DynamicForwardCoordinator)(nil)

func newTunnelHandleID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
