package usecase

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"

	"ssh-client/internal/domain"
	"ssh-client/internal/pkg/safego"
)

const defaultForwardConnLimit = 64

// ForwardRuleRunner starts/stops native (non-plugin) local/remote port forwards.
type ForwardRuleRunner struct {
	client  domain.TunnelChannelDialer
	active  map[string]io.Closer
	sem     chan struct{}
}

// NewForwardRuleRunner creates a forward rule runner for one live SSH session.
func NewForwardRuleRunner(client domain.TunnelChannelDialer) *ForwardRuleRunner {
	return &ForwardRuleRunner{
		client: client,
		active: make(map[string]io.Closer),
		sem:    make(chan struct{}, defaultForwardConnLimit),
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
	ln, err := net.Listen("tcp", net.JoinHostPort(rule.BindAddress, strconv.Itoa(rule.BindPort)))
	if err != nil {
		return fmt.Errorf("listen local forward %s: %w", rule.ID, err)
	}
	r.active[rule.ID] = ln
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
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return
	}
	remote, err := r.client.OpenDirectTCP(ctx, net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		return
	}
	defer remote.Close()
	splice(local, remote)
}

func (r *ForwardRuleRunner) startRemote(ctx context.Context, rule domain.ForwardRule) error {
	ln, err := r.client.ListenTCP(ctx, net.JoinHostPort(rule.BindAddress, strconv.Itoa(rule.BindPort)))
	if err != nil {
		return fmt.Errorf("listen remote forward %s: %w", rule.ID, err)
	}
	r.active[rule.ID] = ln
	safego.Go(func() {
		for {
			remote, err := ln.Accept()
			if err != nil {
				return
			}
			safego.Go(func() { r.spliceRemoteToLocal(remote, rule.TargetHost, rule.TargetPort) })
		}
	})
	return nil
}

func (r *ForwardRuleRunner) spliceRemoteToLocal(remote net.Conn, targetHost string, targetPort int) {
	defer remote.Close()
	local, err := net.Dial("tcp", net.JoinHostPort(targetHost, strconv.Itoa(targetPort)))
	if err != nil {
		return
	}
	defer local.Close()
	splice(remote, local)
}

// Stop closes one active forward rule listener.
func (r *ForwardRuleRunner) Stop(ruleID string) {
	if c, ok := r.active[ruleID]; ok {
		c.Close()
		delete(r.active, ruleID)
	}
}

// StopAll closes all active forward rule listeners.
func (r *ForwardRuleRunner) StopAll() {
	for id := range r.active {
		r.Stop(id)
	}
}
