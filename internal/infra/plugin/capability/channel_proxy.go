package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// ChannelBackendResolver resolves the ChannelPurposeBackend that serves a requested purpose.
// Real backends (exec/tcp-relay/embed-stream) land in Stages 6-8; Stage 3 tests inject a fake.
type ChannelBackendResolver func(purpose string) (domainplugin.ChannelPurposeBackend, error)

// ChannelProxy enforces, per plugin process: the manifest's declared channel purposes,
// maxConcurrent enforced before any channel resource is created (reserve->act->commit, same
// model as TunnelDialProxy), host-monotonic channelId allocation that is never reused, and
// channelId ownership for channel.close (IDOR, via requireChannel). The parentSessionId
// ownership check is a *session* authorization concern and deliberately lives one layer up
// (usecase/plugin_session_rpc.go, via the existing SessionRPCAuthorizer) — infra/plugin must
// never contain session authorization logic (enforced by
// test/unit/plugin/architecture_security_test.go).
type ChannelProxy struct {
	pluginID string
	purposes map[string]struct{}
	max      int
	resolve  ChannelBackendResolver
	audit    domainplugin.ChannelAuditRecorder

	mu           sync.Mutex
	nextID       uint32
	channels     map[uint32]domainplugin.ChannelPurposeBackend
	pendingOpens int
}

// NewChannelProxy creates a channel capability proxy for one plugin process.
func NewChannelProxy(
	pluginID string,
	caps *domainplugin.ChannelCaps,
	resolve ChannelBackendResolver,
	audit domainplugin.ChannelAuditRecorder,
) *ChannelProxy {
	max := domainplugin.DefaultMaxConcurrentChannels
	purposes := make(map[string]struct{})
	if caps != nil {
		if caps.MaxConcurrent > 0 {
			max = caps.MaxConcurrent
		}
		for _, p := range caps.Purposes {
			purposes[strings.TrimSpace(p)] = struct{}{}
		}
	}
	return &ChannelProxy{
		pluginID: pluginID,
		purposes: purposes,
		max:      max,
		resolve:  resolve,
		audit:    audit,
		channels: make(map[uint32]domainplugin.ChannelPurposeBackend),
	}
}

type channelOpenParams struct {
	ParentSessionID string `json:"parentSessionId"`
	Purpose         string `json:"purpose"`
	Hint            string `json:"hint,omitempty"`
}

type channelOpenResult struct {
	ChannelID uint32 `json:"channelId"`
}

type channelCloseParams struct {
	ChannelID uint32 `json:"channelId"`
}

// Open handles channel.open: authorize the parentSessionId (IDOR), validate the requested
// purpose is declared in the manifest, reserve a maxConcurrent slot, authorize+wire the purpose
// backend, then allocate and commit a host-monotonic channelId. The reservation happens strictly
// before any backend call so a plugin cannot force resource creation past its concurrency limit
// (TOCTOU on maxConcurrent is exactly the bug class this ordering prevents).
func (p *ChannelProxy) Open(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil || p.resolve == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	var req channelOpenParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid channel.open params: %w", err)
	}
	req.Purpose = strings.TrimSpace(req.Purpose)
	req.ParentSessionID = strings.TrimSpace(req.ParentSessionID)
	if req.Purpose == "" || req.ParentSessionID == "" {
		return nil, fmt.Errorf("invalid channel.open params: purpose and parentSessionId required")
	}

	if _, ok := p.purposes[req.Purpose]; !ok {
		p.auditOpen(0, req, false, "purpose not declared in manifest")
		return nil, domainplugin.ErrCapabilityDenied
	}

	backend, err := p.resolve(req.Purpose)
	if err != nil || backend == nil {
		p.auditOpen(0, req, false, "purpose backend unavailable")
		return nil, domainplugin.ErrCapabilityDenied
	}

	p.mu.Lock()
	if len(p.channels)+p.pendingOpens >= p.max {
		p.mu.Unlock()
		p.auditOpen(0, req, false, "maxConcurrent reached")
		return nil, domainplugin.ErrRateLimited
	}
	p.pendingOpens++
	p.mu.Unlock()
	committed := false
	defer func() {
		if !committed {
			p.mu.Lock()
			p.pendingOpens--
			p.mu.Unlock()
		}
	}()

	if err := backend.Authorize(req.Purpose, req.ParentSessionID, req.Hint); err != nil {
		p.auditOpen(0, req, false, err.Error())
		return nil, err
	}

	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.mu.Unlock()

	handle := &domainplugin.ChannelHandle{
		ChannelID:       id,
		PluginID:        p.pluginID,
		Purpose:         req.Purpose,
		ParentSessionID: req.ParentSessionID,
		Hint:            req.Hint,
	}
	if err := backend.Wire(ctx, handle); err != nil {
		p.auditOpen(id, req, false, err.Error())
		return nil, err
	}

	p.mu.Lock()
	p.pendingOpens--
	p.channels[id] = backend
	committed = true
	p.mu.Unlock()

	p.auditOpen(id, req, true, "")
	return json.Marshal(channelOpenResult{ChannelID: id})
}

// Close handles channel.close for a channelId owned by this plugin process. Unknown/foreign
// channelIds are rejected before the backend is ever touched (IDOR).
func (p *ChannelProxy) Close(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	if p == nil {
		return nil, domainplugin.ErrCapabilityDenied
	}
	var req channelCloseParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid channel.close params: %w", err)
	}

	backend, err := p.requireChannel(req.ChannelID)
	if err != nil {
		return nil, err
	}

	closeErr := backend.CloseRemote()

	p.mu.Lock()
	delete(p.channels, req.ChannelID)
	p.mu.Unlock()

	p.auditClose(req.ChannelID, closeErr == nil, errString(closeErr))
	if closeErr != nil {
		return nil, closeErr
	}
	return json.Marshal(map[string]bool{"ok": true})
}

// CloseAll closes and drops every channel this proxy owns (process shutdown / crash teardown).
func (p *ChannelProxy) CloseAll() {
	if p == nil {
		return
	}
	p.mu.Lock()
	owned := p.channels
	p.channels = make(map[uint32]domainplugin.ChannelPurposeBackend)
	p.pendingOpens = 0
	p.mu.Unlock()

	for id, backend := range owned {
		closeErr := backend.CloseRemote()
		p.auditClose(id, closeErr == nil, errString(closeErr))
	}
}

func (p *ChannelProxy) requireChannel(channelID uint32) (domainplugin.ChannelPurposeBackend, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	backend, ok := p.channels[channelID]
	if !ok {
		return nil, domainplugin.ErrHandleNotFound
	}
	return backend, nil
}

func (p *ChannelProxy) auditOpen(channelID uint32, req channelOpenParams, success bool, errMsg string) {
	if p.audit == nil {
		return
	}
	p.audit(domainplugin.ChannelAuditEntry{
		Timestamp:       time.Now(),
		PluginID:        p.pluginID,
		Action:          "channel.open",
		ChannelID:       channelID,
		Purpose:         req.Purpose,
		ParentSessionID: req.ParentSessionID,
		Target:          req.Hint,
		Success:         success,
		Error:           errMsg,
	})
}

func (p *ChannelProxy) auditClose(channelID uint32, success bool, errMsg string) {
	if p.audit == nil {
		return
	}
	p.audit(domainplugin.ChannelAuditEntry{
		Timestamp: time.Now(),
		PluginID:  p.pluginID,
		Action:    "channel.close",
		ChannelID: channelID,
		Success:   success,
		Error:     errMsg,
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
