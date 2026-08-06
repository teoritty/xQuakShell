package capability

import (
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ChannelBus fans a session-scoped close out to every plugin process's ChannelProxy, since a
// session's channels can be spread across multiple plugin processes. It owns no channel state
// itself (that stays in each ChannelProxy's own map/mutex) — it is only a registry of
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

// Register tracks a plugin process's ChannelProxy so its channels are reachable from CloseSession.
// processKey identifies the plugin process instance, and is REUSED: a plugin that is stopped and
// started again comes back under the same key.
//
// It claims a free slot and refuses an occupied one rather than overwriting it. An occupied slot
// means the caller is not the process that owns the key: both teardown paths take a process off the
// bus before releasing its key in the host's registry, so whoever legitimately owns the key finds
// the slot empty. The caller that finds it taken is a start that was overtaken — and overwriting
// there would put a dead process's proxy in front of a live one, which is the failure CloseSession
// cannot see, because the bus would answer with a proxy rather than with nothing.
//
// Registering the same proxy twice is idempotent, so a retry is not punished for its own record.
func (b *ChannelBus) Register(processKey string, proxy *ChannelProxy) {
	if b == nil || proxy == nil {
		return
	}
	b.mu.Lock()
	if existing, taken := b.proxies[processKey]; !taken || existing == proxy {
		b.proxies[processKey] = proxy
	}
	b.mu.Unlock()
}

// Unregister drops a plugin process's ChannelProxy on process exit or crash teardown.
//
// The proxy is half of the key. Process keys are reused — a plugin restarts under the same
// pluginID — so a teardown running late for a dead process would otherwise tear the registration
// off the LIVE process that replaced it, and silently cut that process's channels out of the
// CloseSession cascade: the bus would report no proxy for a key that has one.
//
// The check belongs here rather than in the caller because it is a question about this map: the
// host cannot ask "is the proxy under that key still mine" without asking the bus, and a check made
// outside would not be atomic with the delete. (An earlier version of this comment justified the
// placement by a lock inversion. That was wrong — b.mu is a leaf, nothing holds it while taking any
// other lock — and the wrong reason cost a real defect, since it also argued Register out of the
// caller's critical section, where it belongs.)
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
