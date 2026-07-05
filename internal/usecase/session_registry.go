package usecase

import (
	"context"
	"sync"

	"ssh-client/internal/domain"
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

// View runs fn under the read lock IF the session exists. Use for reads that
// need a consistent multi-field snapshot (e.g. reading pluginID + ctx together).
func (r *SessionRegistry) View(id string, fn func(entry *sessionEntry)) bool {
	r.mu.RLock()
	entry, ok := r.sessions[id]
	if !ok {
		r.mu.Unlock()
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
