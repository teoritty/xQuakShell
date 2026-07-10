package usecase

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

const defaultForwardConnLimit = 64

// ForwardRuleRunner starts/stops native (non-plugin) local/remote port forwards.
type ForwardRuleRunner struct {
	mu      sync.Mutex
	client  domain.TunnelChannelDialer
	active  map[string]io.Closer
	limiter domain.ConcurrencyLimiter
}

// NewForwardRuleRunner creates a forward rule runner for one live SSH session.
func NewForwardRuleRunner(client domain.TunnelChannelDialer, limiter domain.ConcurrencyLimiter) *ForwardRuleRunner {
	if limiter == nil {
		panic("usecase: ForwardRuleRunner requires ConcurrencyLimiter")
	}
	return &ForwardRuleRunner{
		client:  client,
		active:  make(map[string]io.Closer),
		limiter: limiter,
	}
}

// Start activates a native forward rule.
func (r *ForwardRuleRunner) Start(ctx context.Context, rule domain.ForwardRule) error {
	switch rule.Kind {
	case domain.ForwardRuleLocal:
		return r.startLocal(ctx, rule)
	case domain.ForwardRuleRemote:
		return r.startRemote(ctx, rule)
	default:
		return fmt.Errorf("ForwardRuleRunner does not handle kind %q (plugin-mediated)", rule.Kind)
	}
}

func (r *ForwardRuleRunner) startLocal(ctx context.Context, rule domain.ForwardRule) error {
	bindAddr := domain.EffectiveBindAddress(rule.BindAddress)
	ln, err := net.Listen("tcp", net.JoinHostPort(bindAddr, strconv.Itoa(rule.BindPort)))
	if err != nil {
		return fmt.Errorf("listen local forward %s: %w", rule.ID, err)
	}
	r.mu.Lock()
	r.active[rule.ID] = ln
	r.mu.Unlock()
	safego.Go(func() {
		for {
			local, err := ln.Accept()
			if err != nil {
				return
			}
			safego.Go(func() { r.spliceLocalToRemote(ctx, local, rule.TargetHost, rule.TargetPort) })
		}
	})
	return nil
}

func (r *ForwardRuleRunner) spliceLocalToRemote(ctx context.Context, local net.Conn, targetHost string, targetPort int) {
	defer local.Close()
	if err := r.limiter.Acquire(ctx); err != nil {
		return
	}
	defer r.limiter.Release()
	remote, err := r.client.OpenDirectTCP(ctx, net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		return
	}
	defer remote.Close()
	splice(local, remote)
}

func (r *ForwardRuleRunner) startRemote(ctx context.Context, rule domain.ForwardRule) error {
	bindAddr := domain.EffectiveBindAddress(rule.BindAddress)
	ln, err := r.client.ListenTCP(ctx, net.JoinHostPort(bindAddr, strconv.Itoa(rule.BindPort)))
	if err != nil {
		return fmt.Errorf("listen remote forward %s: %w", rule.ID, err)
	}
	r.mu.Lock()
	r.active[rule.ID] = ln
	r.mu.Unlock()
	safego.Go(func() {
		for {
			remote, err := ln.Accept()
			if err != nil {
				return
			}
			safego.Go(func() { r.spliceRemoteToLocal(ctx, remote, rule.TargetHost, rule.TargetPort) })
		}
	})
	return nil
}

func (r *ForwardRuleRunner) spliceRemoteToLocal(ctx context.Context, remote net.Conn, targetHost string, targetPort int) {
	defer remote.Close()
	if err := r.limiter.Acquire(ctx); err != nil {
		return
	}
	defer r.limiter.Release()
	local, err := net.Dial("tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		return
	}
	defer local.Close()
	splice(remote, local)
}

// Stop closes one active forward rule listener.
func (r *ForwardRuleRunner) Stop(ruleID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.active[ruleID]; ok {
		c.Close()
		delete(r.active, ruleID)
	}
}

// StopAll closes all active forward rule listeners.
func (r *ForwardRuleRunner) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.active {
		c.Close()
		delete(r.active, id)
	}
}
