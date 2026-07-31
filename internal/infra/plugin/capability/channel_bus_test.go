package capability

import "testing"

// TestUnregisterOnlyRemovesItsOwnProxy pins the rule that makes the bus safe against key reuse.
//
// Process keys are reused: a plugin that crashes and is restarted comes back under the same
// pluginID. Both teardown paths in the host used to unregister by key alone, and they run late by
// nature — after the reaper has reaped and the resources are closed — so a dead process could
// deregister the live one that had already replaced it. Nothing failed loudly when that happened:
// the process stayed up and served RPC, and only the session-close cascade quietly stopped reaching
// its channels, leaving remote ends alive with no owner.
//
// The identity check cannot live in the host instead, because the host would have to hold its own
// lock across the bus call to make the check and the delete atomic, nesting the two mutexes against
// the paths that take them one after the other.
func TestUnregisterOnlyRemovesItsOwnProxy(t *testing.T) {
	const key = "com.example.plugin"
	bus := NewChannelBus()
	dead := NewChannelProxy("com.example.plugin", nil, nil, nil)
	live := NewChannelProxy("com.example.plugin", nil, nil, nil)

	bus.Register(key, dead)
	bus.Register(key, live) // the restart claims the key

	// The crashed process finishes its teardown now, after the restart.
	bus.Unregister(key, dead)

	bus.mu.RLock()
	got, present := bus.proxies[key]
	bus.mu.RUnlock()
	if !present {
		t.Fatal("a dead process's teardown deregistered the live process that replaced it; its " +
			"channels would no longer be reached by CloseSession")
	}
	if got != live {
		t.Fatal("the bus holds a proxy that is neither the live one nor absent")
	}

	// And the live process can still take itself off when its own turn comes.
	bus.Unregister(key, live)
	bus.mu.RLock()
	_, present = bus.proxies[key]
	bus.mu.RUnlock()
	if present {
		t.Fatal("a process must be able to deregister its own proxy")
	}
}
