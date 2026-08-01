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

	// The schedule is the production one. A process has two teardown paths that both take it off the
	// bus — Stop's finalizeProcess and the reaper's waitProcess — and each releases the key in the
	// host's registry only after unregistering, which is what lets the restart in. The second path
	// then arrives late, after the restart has claimed both the key and the bus slot.
	bus.Register(key, dead)
	bus.Unregister(key, dead) // first teardown path for the dead process
	bus.Register(key, live)   // the restart claims the freed key
	bus.Unregister(key, dead) // second teardown path, arriving after the restart

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

// TestRegisterDoesNotOverwriteAnotherProcessesProxy is the same rule from the other side, and it
// closes a defect the identity check on Unregister alone left open.
//
// The schedule: a Start is overtaken — its process is stopped, the key is released, and a second
// Start claims it and registers. The first Start then reaches its own registration and, overwriting
// by key, puts its dead proxy in front of the live one. Whatever happens next, the live process ends
// up unreachable from CloseSession: either the stale record stays, or the overtaken Start withdraws
// it on its way out and takes the slot with it. Both leave a running plugin's channels outside the
// session-close cascade, which is exactly what Unregister's identity check exists to prevent.
//
// Refusing the occupied slot is safe rather than merely cautious: both teardown paths take a
// process off the bus BEFORE releasing its key in the host's registry, so a start that legitimately
// owns the key always finds the slot empty. Only an overtaken one finds it taken.
func TestRegisterDoesNotOverwriteAnotherProcessesProxy(t *testing.T) {
	const key = "com.example.plugin"
	bus := NewChannelBus()
	live := NewChannelProxy("com.example.plugin", nil, nil, nil)
	overtaken := NewChannelProxy("com.example.plugin", nil, nil, nil)

	bus.Register(key, live)
	bus.Register(key, overtaken) // the overtaken start, arriving late

	bus.mu.RLock()
	got := bus.proxies[key]
	bus.mu.RUnlock()
	if got != live {
		t.Fatal("an overtaken start overwrote the live process's proxy; the live process's channels " +
			"would be closed by nobody, and CloseSession would reach a dead process instead")
	}

	// Re-registering the same proxy must stay harmless — a record's owner may write it twice.
	bus.Register(key, live)
	bus.mu.RLock()
	got = bus.proxies[key]
	bus.mu.RUnlock()
	if got != live {
		t.Fatal("a process must be able to re-register its own proxy")
	}
}
