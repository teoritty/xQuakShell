package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// TunnelDialSlotReleaser releases a per-plugin tunnel.dial handle and concurrency slot.
type TunnelDialSlotReleaser func(pluginID, sessionID, tunnelID string)

type tunnelHandleOwner struct {
	sessionID string
	pluginID  string
}

type sessionDynamicForward struct {
	sessionID string
	service   *TunnelDynamicService
	listeners map[string]net.Listener
	rules     map[string]domain.ForwardRule
	cancel    context.CancelFunc
}

// DynamicForwardCoordinator manages dynamic (-D) forward rules bound to SSH sessions.
type DynamicForwardCoordinator struct {
	mu              sync.Mutex
	sessions        map[string]*sessionDynamicForward
	localOwners     map[string]tunnelHandleOwner
	tunnelOwners    map[string]tunnelHandleOwner
	channelTimers   map[string]*time.Timer
	dialSlotRelease TunnelDialSlotReleaser
	notify          PluginTunnelNotifier
	starter         PluginTunnelStarter
	tunnelGrant     PluginTunnelGrantReader
}

// NewDynamicForwardCoordinator creates a coordinator for plugin-mediated dynamic forwards.
func NewDynamicForwardCoordinator(notify PluginTunnelNotifier, starter PluginTunnelStarter) *DynamicForwardCoordinator {
	return &DynamicForwardCoordinator{
		sessions:      make(map[string]*sessionDynamicForward),
		localOwners:   make(map[string]tunnelHandleOwner),
		tunnelOwners:  make(map[string]tunnelHandleOwner),
		channelTimers: make(map[string]*time.Timer),
		notify:        notify,
		starter:       starter,
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

// SetDialSlotReleaser wires infra dial-proxy slot release on bind timeout/close.
func (c *DynamicForwardCoordinator) SetDialSlotReleaser(release TunnelDialSlotReleaser) {
	if c != nil {
		c.dialSlotRelease = release
	}
}

// StartSession activates enabled dynamic forward rules for a live SSH session.
func (c *DynamicForwardCoordinator) StartSession(parentCtx context.Context, sessionID string, dialer domain.TunnelChannelDialer, rules []domain.ForwardRule) {
	if c == nil || dialer == nil {
		return
	}
	if c.tunnelGrant == nil {
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	svc := NewTunnelDynamicService(dialer, c.notify)
	svc.SetPreBindEvictHook(func(_ string, localConnID string) {
		c.mu.Lock()
		delete(c.localOwners, localConnID)
		c.mu.Unlock()
	})
	sf := &sessionDynamicForward{
		sessionID: sessionID,
		service:   svc,
		listeners: make(map[string]net.Listener),
		rules:     make(map[string]domain.ForwardRule),
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
		sf.rules[rule.ID] = rule
		if err := c.startDynamicRule(ctx, sf, rule); err != nil {
			delete(sf.rules, rule.ID)
		}
	}
}

func (c *DynamicForwardCoordinator) startDynamicRule(ctx context.Context, sf *sessionDynamicForward, rule domain.ForwardRule) error {
	if !c.tunnelGrant.IsTunnelProviderGranted(rule.PluginID) {
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
	if err := sf.service.RegisterLocal(ctx, rule.PluginID, rule.ID, rule.ProviderID, localConnID, conn); err != nil {
		conn.Close()
		return
	}

	c.mu.Lock()
	c.localOwners[localConnID] = tunnelHandleOwner{sessionID: sf.sessionID, pluginID: rule.PluginID}
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) markLocalBound(localConnID string) {
	c.mu.Lock()
	delete(c.localOwners, localConnID)
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) releaseTunnelOwner(tunnelID string) {
	c.mu.Lock()
	if timer, ok := c.channelTimers[tunnelID]; ok {
		timer.Stop()
		delete(c.channelTimers, tunnelID)
	}
	delete(c.tunnelOwners, tunnelID)
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) armChannelTimeout(pluginID, tunnelID string, sf *sessionDynamicForward) {
	c.mu.Lock()
	if old, ok := c.channelTimers[tunnelID]; ok {
		old.Stop()
	}
	timer := time.AfterFunc(domainplugin.PreBindTunnelTimeout, func() {
		_ = sf.service.CloseChannel(tunnelID)
		c.releaseTunnelOwner(tunnelID)
		if c.dialSlotRelease != nil {
			c.dialSlotRelease(pluginID, sf.sessionID, tunnelID)
		}
	})
	c.channelTimers[tunnelID] = timer
	c.mu.Unlock()
}

func (c *DynamicForwardCoordinator) clearSessionOwners(sessionID string) {
	for id, owner := range c.localOwners {
		if owner.sessionID == sessionID {
			delete(c.localOwners, id)
		}
	}
	for id, owner := range c.tunnelOwners {
		if owner.sessionID == sessionID {
			if timer, ok := c.channelTimers[id]; ok {
				timer.Stop()
				delete(c.channelTimers, id)
			}
			if c.dialSlotRelease != nil {
				c.dialSlotRelease(owner.pluginID, sessionID, id)
			}
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
	if sf.service != nil {
		sf.service.CloseAllPreBind()
	}
}

func (c *DynamicForwardCoordinator) sessionForRule(pluginID, ruleID string) (*sessionDynamicForward, domain.ForwardRule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sf := range c.sessions {
		rule, ok := sf.rules[ruleID]
		if !ok || rule.PluginID != pluginID {
			continue
		}
		return sf, rule, nil
	}
	return nil, domain.ForwardRule{}, domainplugin.ErrTunnelNotFound
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

func newTunnelHandleID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
