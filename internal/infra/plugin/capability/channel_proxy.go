package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ChannelBackendResolver resolves the ChannelPurposeBackend that serves a requested purpose for
// one plugin process. It takes only the purpose because everything else a backend needs — the
// manifest, the parent session — is closed over when the resolver is built, per process, by
// HostConfig.ChannelResolverFor. Real backends (exec/tcp-relay/embed-stream) land in Stages 6-8;
// Stage 3 tests inject a fake.
//
// It must return a fresh backend per call: the backends are stateful and single-use (they store
// the resolved argv/target/tunnelId during Authorize), so a shared instance lets the second
// channel.open overwrite the first's target.
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
	opener       domainplugin.ChannelDataPathOpener
	nextID       uint32
	channels     map[uint32]*channelEntry
	pendingOpens int
}

// AttachDataPathOpener supplies the bus this proxy opens channel data paths on. It exists
// because the proxy and the ipc conn are mutually dependent (the conn's request handler routes
// channel.open here), so the cycle has to be broken by assignment rather than construction.
//
// Composition-root only, exactly once, before the plugin process is served. It is not a general
// setter: a second call is a wiring bug, not a reconfiguration.
func (p *ChannelProxy) AttachDataPathOpener(opener domainplugin.ChannelDataPathOpener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opener = opener
}

// channelEntry pairs an open channel's backend with the parentSessionId it was opened for, so
// CloseSession can select exactly the channels bound to a closing session without a second
// tracking map (ADR-011 §Session lifecycle coupling).
type channelEntry struct {
	backend domainplugin.ChannelPurposeBackend
	// data is closed on every teardown path alongside the backend. Closing only the backend
	// leaves its pump goroutines parked in Recv forever, holding the channel and its queue.
	data domainplugin.ChannelDataPath
	// cancelWire ends the context handed to backend.Wire, which every pump goroutine that Wire
	// started gates its frames on.
	cancelWire      context.CancelFunc
	parentSessionID string
}

// closeEntry tears down all of a channel: the host-owned far end, the bus-side data path, and
// the context its pumps run under. Order matters — the backend stops touching the path first,
// then the path unblocks anything parked on it, then the context releases anything gated on it.
func closeEntry(entry *channelEntry) error {
	closeErr := entry.backend.CloseRemote()
	if entry.data != nil {
		_ = entry.data.Close()
	}
	if entry.cancelWire != nil {
		entry.cancelWire()
	}
	return closeErr
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
		channels: make(map[uint32]*channelEntry),
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

	slog.Debug("channel.open: calling Authorize", "pluginId", p.pluginID, "purpose", req.Purpose, "hint", req.Hint)
	if err := backend.Authorize(req.Purpose, req.ParentSessionID, req.Hint); err != nil {
		slog.Debug("channel.open: Authorize failed", "pluginId", p.pluginID, "purpose", req.Purpose, "err", err.Error())
		p.auditOpen(0, req, false, err.Error())
		return nil, err
	}
	slog.Debug("channel.open: Authorize OK", "pluginId", p.pluginID, "purpose", req.Purpose)

	p.mu.Lock()
	opener := p.opener
	p.nextID++
	id := p.nextID
	p.mu.Unlock()

	if opener == nil {
		p.auditOpen(id, req, false, "channel data path opener not attached")
		return nil, domainplugin.ErrCapabilityDenied
	}

	// The data path is opened before the backend is wired, so a channel is never handed to a
	// backend without one. Reporting the failure here — rather than returning a channelId that
	// cannot carry bytes — is the difference between the plugin learning of it in this call and
	// learning of it when the host kills it over the first frame.
	slog.Debug("channel.open: calling OpenDataPath", "pluginId", p.pluginID, "purpose", req.Purpose, "channelId", id)
	data, err := opener.OpenDataPath(id, req.Purpose)
	if err != nil {
		slog.Debug("channel.open: OpenDataPath failed", "pluginId", p.pluginID, "err", err.Error())
		p.auditOpen(id, req, false, err.Error())
		return nil, err
	}
	slog.Debug("channel.open: OpenDataPath OK", "pluginId", p.pluginID, "channelId", id)
	// Until the channel is committed, every exit path owns closing the data path: otherwise a
	// rejected open leaves a registered channel on the bus forever.
	wired := false
	defer func() {
		if !wired {
			_ = data.Close()
		}
	}()

	handle, err := domainplugin.NewChannelHandle(id, p.pluginID, req.Purpose, req.ParentSessionID, req.Hint, data)
	if err != nil {
		p.auditOpen(id, req, false, err.Error())
		return nil, err
	}

	// Wire's context is the channel's lifetime, not this request's. It is deliberately not
	// derived from ctx's cancellation: ctx belongs to the channel.open RPC and the ipc layer
	// cancels it as soon as that call is answered, whereas the pump goroutines Wire starts gate
	// every frame they move on the context they were handed. Passing ctx straight through would
	// leave every channel reporting success and then moving nothing, its pumps having exited
	// before the reply reached the plugin. Values (tracing, deadlines carried as values) are
	// kept; only the cancellation is cut.
	wireCtx, cancelWire := context.WithCancel(context.WithoutCancel(ctx))
	defer func() {
		if !wired {
			cancelWire()
		}
	}()

	slog.Debug("channel.open: calling Wire", "pluginId", p.pluginID, "purpose", req.Purpose, "channelId", id)
	if err := backend.Wire(wireCtx, handle); err != nil {
		slog.Debug("channel.open: Wire failed", "pluginId", p.pluginID, "channelId", id, "err", err.Error())
		p.auditOpen(id, req, false, err.Error())
		return nil, err
	}
	slog.Debug("channel.open: Wire OK", "pluginId", p.pluginID, "channelId", id)

	p.mu.Lock()
	p.pendingOpens--
	p.channels[id] = &channelEntry{
		backend:         backend,
		data:            data,
		cancelWire:      cancelWire,
		parentSessionID: req.ParentSessionID,
	}
	committed = true
	wired = true
	p.mu.Unlock()

	p.auditOpen(id, req, true, "")
	slog.Debug("channel.open: returning channelId to plugin", "pluginId", p.pluginID, "channelId", id)
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

	entry, err := p.requireChannel(req.ChannelID)
	if err != nil {
		return nil, err
	}

	closeErr := closeEntry(entry)

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
	p.channels = make(map[uint32]*channelEntry)
	p.pendingOpens = 0
	p.mu.Unlock()

	for id, entry := range owned {
		closeErr := closeEntry(entry)
		p.auditClose(id, closeErr == nil, errString(closeErr))
	}
}

// CloseSession closes and drops every channel this proxy owns that is bound to sessionID,
// leaving channels of other sessions on the same plugin process untouched. This is the
// one-directional session->channels cascade (ADR-011 §Session lifecycle coupling); closing a
// single channel must never reach back up to affect the parent session or its siblings, and
// this method itself never touches session state — only its own channels map.
func (p *ChannelProxy) CloseSession(sessionID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	var owned []struct {
		id    uint32
		entry *channelEntry
	}
	for id, entry := range p.channels {
		if entry.parentSessionID == sessionID {
			owned = append(owned, struct {
				id    uint32
				entry *channelEntry
			}{id, entry})
			delete(p.channels, id)
		}
	}
	p.mu.Unlock()

	for _, o := range owned {
		closeErr := closeEntry(o.entry)
		p.auditClose(o.id, closeErr == nil, errString(closeErr))
	}
}

func (p *ChannelProxy) requireChannel(channelID uint32) (*channelEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.channels[channelID]
	if !ok {
		return nil, domainplugin.ErrHandleNotFound
	}
	return entry, nil
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
