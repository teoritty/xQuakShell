package main

import (
	"sync"
	"testing"
)

// TestDiscoveryEmitHolderIsInertUntilWiredThenForwardsBoth covers the one piece of behaviour the
// composition root adds to discovery: the tree-changed callback is late-bound, because the emit
// coalescer is built before the AppAPI that owns the Wails context exists.
//
// Both halves matter. Before wiring, an emit must be a no-op rather than a nil call — nothing can
// receive it yet. After wiring, the connection AND the node must arrive intact: coalescing windows
// are per node, and a callback that dropped the node would leave the frontend able only to refetch
// the whole tree.
func TestDiscoveryEmitHolderIsInertUntilWiredThenForwardsBoth(t *testing.T) {
	holder := newDiscoveryEmitHolder()
	holder.notify("conn-1", "docker") // must not panic

	type emitted struct{ connectionID, nodeID string }
	var got []emitted
	holder.set(func(connectionID, nodeID string) {
		got = append(got, emitted{connectionID, nodeID})
	})

	holder.notify("conn-1", "docker")
	// The connection root is a legitimate node id and must survive as an empty string.
	holder.notify("conn-2", "")

	want := []emitted{{"conn-1", "docker"}, {"conn-2", ""}}
	if len(got) != len(want) {
		t.Fatalf("expected %d emits, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emit %d: want %+v, got %+v", i, want[i], got[i])
		}
	}
}

// TestDiscoveryEmitHolderSurvivesConcurrentWiringAndEmits: publishes arrive on plugin RPC
// goroutines and trailing coalesced emits arrive on timer goroutines, both while composition may
// still be running. Under -race this fails loudly if the holder ever reads the callback unguarded.
func TestDiscoveryEmitHolderSurvivesConcurrentWiringAndEmits(t *testing.T) {
	holder := newDiscoveryEmitHolder()

	var mu sync.Mutex
	var count int
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			holder.notify("conn-1", "docker")
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		holder.set(func(string, string) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}()
	wg.Wait()

	holder.notify("conn-1", "docker")
	mu.Lock()
	defer mu.Unlock()
	if count == 0 {
		t.Fatal("expected the emit after wiring to reach the callback")
	}
}
