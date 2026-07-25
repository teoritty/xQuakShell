package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestPluginTerminalBridge_WriteDoesNotBlockOnParkedSend verifies that Write
// returns quickly even while a previous batch's RPC is parked (e.g. the
// plugin stopped reading stdin). Before the fix, the 5s host->plugin RPC ran
// under b.mu, so every subsequent keystroke blocked for up to 5s. After the
// fix, sendBatch runs outside b.mu, so Write only ever touches the small
// in-memory buffer and returns immediately.
func TestPluginTerminalBridge_WriteDoesNotBlockOnParkedSend(t *testing.T) {
	entered := make(chan string, 4)
	release := make(chan struct{})

	b := &pluginTerminalBridge{
		notify: func(_ context.Context, _ string, params json.RawMessage) error {
			var m map[string]string
			if err := json.Unmarshal(params, &m); err != nil {
				t.Errorf("unmarshal params: %v", err)
			}
			data, err := base64.StdEncoding.DecodeString(m["dataBase64"])
			if err != nil {
				t.Errorf("decode payload: %v", err)
			}
			entered <- string(data)
			<-release
			return nil
		},
	}

	// Write a small batch below the size threshold: it arms the batching
	// timer instead of sending synchronously.
	if err := b.Write([]byte("first")); err != nil {
		t.Fatalf("Write(first): %v", err)
	}

	// Wait for the timer's flush to actually enter the RPC and park there,
	// simulating a plugin that stopped reading its stdin.
	select {
	case got := <-entered:
		if got != "first" {
			t.Fatalf("first send payload = %q, want %q", got, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first batch to reach notify")
	}

	// While that send is parked on sendMu inside notify, Write must still
	// return immediately: it only needs b.mu to append to the buffer.
	start := time.Now()
	if err := b.Write([]byte("second")); err != nil {
		t.Fatalf("Write(second): %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Write blocked for %v while a send was parked; want it to return near-instantly", elapsed)
	}

	// Unblock the parked send so the test can clean up without leaking
	// goroutines, and drain the eventual second flush.
	close(release)

	select {
	case got := <-entered:
		if got != "second" {
			t.Fatalf("second send payload = %q, want %q", got, "second")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second batch to reach notify")
	}
}

// TestPluginTerminalBridge_SendsAreSerializedInOrder verifies that moving the
// RPC out from under b.mu did not reintroduce reordering: two sends that
// overlap in time must still reach notify in the order their batches were
// taken, never interleaved.
//
// Determinism: rather than relying on scheduling luck, the test proves
// contention structurally. It starts the first send and blocks until notify
// confirms it is actually inside the RPC (holding sendMu). Only then does it
// start the second send, which therefore *cannot* reach notify until the
// first is released and sendMu is free — Go's mutex semantics guarantee
// this, not timing. So the second "entered" receive can only ever observe
// the second batch after the first has been released, with no sleep and no
// race window.
func TestPluginTerminalBridge_SendsAreSerializedInOrder(t *testing.T) {
	entered := make(chan string, 2)
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})

	var mu sync.Mutex
	var order []string

	b := &pluginTerminalBridge{
		notify: func(_ context.Context, _ string, params json.RawMessage) error {
			var m map[string]string
			if err := json.Unmarshal(params, &m); err != nil {
				t.Errorf("unmarshal params: %v", err)
			}
			data, err := base64.StdEncoding.DecodeString(m["dataBase64"])
			if err != nil {
				t.Errorf("decode payload: %v", err)
			}
			id := string(data)

			mu.Lock()
			order = append(order, id)
			mu.Unlock()
			entered <- id

			if id == "first" {
				<-releaseFirst
			} else {
				<-releaseSecond
			}
			return nil
		},
	}

	go func() { _ = b.sendBatch([]byte("first")) }()

	select {
	case got := <-entered:
		if got != "first" {
			t.Fatalf("first entered = %q, want %q", got, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first send to enter notify")
	}

	// At this point sendMu is provably held by the first send. Starting the
	// second send now guarantees genuine contention on sendMu.
	go func() { _ = b.sendBatch([]byte("second")) }()

	// Release the first send. The second cannot have entered notify yet:
	// it is blocked acquiring sendMu, which the first send still holds.
	close(releaseFirst)

	select {
	case got := <-entered:
		if got != "second" {
			t.Fatalf("second entered = %q, want %q", got, "second")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second send to enter notify")
	}
	close(releaseSecond)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("send order = %v, want [first second]", order)
	}
}
