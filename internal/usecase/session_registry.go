package usecase

import (
	"context"
	"sync"

	"xquakshell/internal/domain"
)

// sessionEntry holds runtime state for a single session (tab).
type sessionEntry struct {
	info                domain.ConnectionSession
	ctx                 context.Context
	cancel              context.CancelFunc
	sshClient           domain.SSHClient
	remoteFS            domain.RemoteFS
	ptyBridge           domain.TerminalPTYBridge
	ptyCols             uint32
	ptyRows             uint32
	hostKeyInfo         *domain.HostKeyInfo
	connectionID        string
	pluginID            string
	pluginOutput        chan []byte
	pluginTerminalReady bool
	sessionSurface      string
	embedDescriptor     *domain.SessionEmbedDescriptor
	forwardRunner       *ForwardRuleRunner
	readyOnce           sync.Once
	readyCh             chan struct{}
}

func newSessionEntry(info domain.ConnectionSession, ctx context.Context, cancel context.CancelFunc, connectionID string) *sessionEntry {
	return &sessionEntry{
		info:         info,
		ctx:          ctx,
		cancel:       cancel,
		connectionID: connectionID,
		readyCh:      make(chan struct{}),
	}
}

func (e *sessionEntry) signalReadyIfTerminal(state domain.SessionState) {
	switch state {
	case domain.SessionReady, domain.SessionError, domain.SessionClosed:
		e.readyOnce.Do(func() { close(e.readyCh) })
	}
}

// SessionRegistry is the single owner of session runtime state.
//
// WHY THIS FILE EXISTS (read before touching m.sessions anywhere else):
// SessionManager used to let five different concerns (SSH connect, PTY/IO,
// plugin-session protocol, embed-tunnel protocol, lifecycle) reach into the
// same `map[string]*sessionEntry` + `sync.RWMutex` directly. That made every
// change risk a data race or a lock-ordering bug, and turned SessionManager
// into a God Object (46 methods on one type, see ADR-009).
//
// The rule going forward: NOTHING outside this file reads or writes
// `sessions` or takes `mu` directly. Every other component (SSHConnector,
// SessionIOService, PluginSessionBridge, EmbedTunnelService,
// SessionLifecycleService) is handed a *SessionRegistry and goes through its
// methods only. If you are about to add `m.mu.Lock()` anywhere else in
// package usecase for session state — stop, that state belongs here instead.
type SessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

// NewSessionRegistry creates an empty session registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{sessions: make(map[string]*sessionEntry)}
}

// Put registers a new session entry. Callers must not retain unsynchronized
// references to fields inside entry after calling Put; use the accessor
// methods below instead.
func (r *SessionRegistry) Put(id string, entry *sessionEntry) {
	r.mu.Lock()
	r.sessions[id] = entry
	r.mu.Unlock()
}

// Get returns the session entry for id.
func (r *SessionRegistry) Get(id string) (*sessionEntry, bool) {
	r.mu.RLock()
	entry, ok := r.sessions[id]
	r.mu.RUnlock()
	return entry, ok
}

// Delete removes and returns the session entry for id.
func (r *SessionRegistry) Delete(id string) (*sessionEntry, bool) {
	r.mu.Lock()
	entry, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	r.mu.Unlock()
	return entry, ok
}

// All returns a snapshot slice; safe to range over without holding the lock.
func (r *SessionRegistry) All() []*sessionEntry {
	r.mu.RLock()
	result := make([]*sessionEntry, 0, len(r.sessions))
	for _, entry := range r.sessions {
		result = append(result, entry)
	}
	r.mu.RUnlock()
	return result
}

// Mutate runs fn under the write lock IF the session still exists (guards
// against acting on a session that was closed concurrently). Prefer this
// over Get+manual locking for any read-modify-write on entry fields.
func (r *SessionRegistry) Mutate(id string, fn func(entry *sessionEntry)) bool {
	r.mu.Lock()
	entry, ok := r.sessions[id]
	if !ok {
		r.mu.Unlock()
		return false
	}
	fn(entry)
	r.mu.Unlock()
	return true
}

// CompareAndTransition atomically moves the session from `from` state to `to`
// state, running fn (if non-nil) under the same write lock to update any
// other fields that must change together with the state (error message,
// hostKeyInfo, etc.). Returns false if the session doesn't exist or is not
// currently in `from` state — in which case nothing is mutated.
//
// WHY THIS EXISTS (ADR-009 follow-up): a naive Get-check-then-Mutate sequence
// has a TOCTOU window between the check and the mutation — two concurrent
// callers can both pass the check before either one mutates. The original
// pre-decomposition code avoided this by holding a single mu.Lock() across
// both the check and the write; this method restores that guarantee without
// reintroducing a shared lock outside SessionRegistry.
func (r *SessionRegistry) CompareAndTransition(id string, from, to domain.SessionState, fn func(entry *sessionEntry)) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.sessions[id]
	if !ok || entry.info.State != from {
		return false
	}
	entry.info.State = to
	if fn != nil {
		fn(entry)
	}
	return true
}

// View runs fn under the read lock IF the session exists. Use for reads that
// need a consistent multi-field snapshot (e.g. reading pluginID + ctx together).
func (r *SessionRegistry) View(id string, fn func(entry *sessionEntry)) bool {
	r.mu.RLock()
	entry, ok := r.sessions[id]
	if !ok {
		r.mu.RUnlock()
		return false
	}
	fn(entry)
	r.mu.RUnlock()
	return true
}

// IDs returns all registered session IDs.
func (r *SessionRegistry) IDs() []string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	return ids
}

// ProtocolForSession returns the connection protocol a session speaks. Discovery needs it to
// decide which plugins a connection may be announced to (parentProtocols, ADR-014), and this is
// the only place that fact is recorded.
func (r *SessionRegistry) ProtocolForSession(id string) (string, bool) {
	r.mu.RLock()
	entry, ok := r.sessions[id]
	if !ok {
		r.mu.RUnlock()
		return "", false
	}
	protocol := entry.info.Protocol
	r.mu.RUnlock()
	return protocol, true
}

// StillRegistered reports whether the session id is in the registry.
func (r *SessionRegistry) StillRegistered(id string) bool {
	r.mu.RLock()
	_, ok := r.sessions[id]
	r.mu.RUnlock()
	return ok
}

// WaitReady returns a channel that closes when the session reaches a terminal
// readiness state (ready, error, or closed).
func (r *SessionRegistry) WaitReady(sessionID string) (<-chan struct{}, error) {
	entry, ok := r.Get(sessionID)
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return entry.readyCh, nil
}
