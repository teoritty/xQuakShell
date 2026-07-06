package usecase

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	domainplugin "ssh-client/internal/domain/plugin"
)

const MaxConcurrentAuthAttempts = 4

// PluginAuthAttempt binds an in-flight auth attempt to exactly one plugin and connection.
type PluginAuthAttempt struct {
	ID           string
	PluginID     string
	ConnectionID string
	AuthMethodID string
}

// PluginAuthAttemptRegistry tracks in-flight auth attempts for RPC authorization.
type PluginAuthAttemptRegistry struct {
	mu       sync.Mutex
	attempts map[string]PluginAuthAttempt
	perPlugin map[string]int
}

// NewPluginAuthAttemptRegistry creates an empty auth attempt registry.
func NewPluginAuthAttemptRegistry() *PluginAuthAttemptRegistry {
	return &PluginAuthAttemptRegistry{
		attempts:  make(map[string]PluginAuthAttempt),
		perPlugin: make(map[string]int),
	}
}

// Begin registers a new auth attempt for the given plugin and connection.
func (r *PluginAuthAttemptRegistry) Begin(pluginID, connectionID, authMethodID string) (PluginAuthAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.perPlugin[pluginID] >= MaxConcurrentAuthAttempts {
		return PluginAuthAttempt{}, domainplugin.ErrAuthProviderBusy
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return PluginAuthAttempt{}, err
	}
	a := PluginAuthAttempt{
		ID: hex.EncodeToString(buf), PluginID: pluginID,
		ConnectionID: connectionID, AuthMethodID: authMethodID,
	}
	r.attempts[a.ID] = a
	r.perPlugin[pluginID]++
	return a, nil
}

// Authorize returns ErrAuthAttemptNotBound when the attempt is missing or not owned by the caller.
func (r *PluginAuthAttemptRegistry) Authorize(pluginID, attemptID, authMethodID string) error {
	r.mu.Lock()
	a, ok := r.attempts[attemptID]
	r.mu.Unlock()
	if !ok || a.PluginID != pluginID || a.AuthMethodID != authMethodID {
		return domainplugin.ErrAuthAttemptNotBound
	}
	return nil
}

var _ domainplugin.AuthAttemptAuthorizer = (*PluginAuthAttemptRegistry)(nil)

// End removes an auth attempt from the registry.
func (r *PluginAuthAttemptRegistry) End(attemptID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.attempts[attemptID]
	if !ok {
		return
	}
	delete(r.attempts, attemptID)
	if r.perPlugin[a.PluginID] > 0 {
		r.perPlugin[a.PluginID]--
	}
}

// Lookup returns the attempt when present.
func (r *PluginAuthAttemptRegistry) Lookup(attemptID string) (PluginAuthAttempt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.attempts[attemptID]
	return a, ok
}
