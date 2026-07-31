package capability

import (
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ChannelBus fans a session-scoped close out to every plugin process's ChannelProxy, since a
// session's channels can be spread across multiple plugin processes. It owns no channel state
// itself (that stays in each ChannelProxy's own map/mutex, Stage 3) — it is only a registry of
// "which processes currently exist," constructed once at the composition root (main_plugins.go)
// and populated as plugin processes come up.
type ChannelBus struct {
	mu      sync.RWMutex
	proxies map[string]*ChannelProxy
}

// NewChannelBus creates an empty channel bus.
func NewChannelBus() *ChannelBus {
	return &ChannelBus{proxies: make(map[string]*ChannelProxy)}
}

// Register tracks a plugin process's ChannelProxy so its channels are reachable from
// CloseSession. processKey should uniquely identify the plugin process instance (e.g. pluginID,
// or pluginID+generation if a process can restart while still holding this key).
func (b *ChannelBus) Register(processKey string, proxy *ChannelProxy) {
	if b == nil || proxy == nil {
		return
	}
	b.mu.Lock()
	b.proxies[processKey] = proxy
	b.mu.Unlock()
}

// Unregister drops a plugin process's ChannelProxy (process exit/crash teardown, Stage 4b).
//
// The proxy is half of the key. Process keys are reused — a plugin restarts under the same
// pluginID — so a teardown running late for a dead process would otherwise tear the registration
// off the LIVE process that replaced it, and silently cut that process's channels out of the
// CloseSession cascade: the bus would report no proxy for a key that has one. Deleting only on
// identity makes that impossible. The alternative, having the host hold its own lock across this
// call so the check and the delete are atomic, would nest the two mutexes in the opposite order
// from the teardown paths that already take them one after the other.
func (b *ChannelBus) Unregister(processKey string, proxy *ChannelProxy) {
	if b == nil || proxy == nil {
		return
	}
	b.mu.Lock()
	if b.proxies[processKey] == proxy {
		delete(b.proxies, processKey)
	}
	b.mu.Unlock()
}

// CloseSession closes every channel bound to sessionID across every registered plugin process.
// Implements domainplugin.ChannelSessionCloser.
func (b *ChannelBus) CloseSession(sessionID string) {
	if b == nil {
		return
	}
	b.mu.RLock()
	proxies := make([]*ChannelProxy, 0, len(b.proxies))
	for _, p := range b.proxies {
		proxies = append(proxies, p)
	}
	b.mu.RUnlock()

	for _, p := range proxies {
		p.CloseSession(sessionID)
	}
}

var _ domainplugin.ChannelSessionCloser = (*ChannelBus)(nil)
