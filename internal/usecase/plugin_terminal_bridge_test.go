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

// parkedBridge builds a bridge whose notify records every delivered payload
// and blocks until the returned release channel is closed for the first call
// only. Subsequent calls return immediately.
func parkedBridge(t *testing.T) (b *pluginTerminalBridge, entered <-chan string, delivered func() []string, release func()) {
	t.Helper()

	enteredCh := make(chan string, 8)
	releaseCh := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var got []string
	first := true

	b = &pluginTerminalBridge{
		notify: func(_ context.Context, method string, params json.RawMessage) error {
			var m map[string]string
			if err := json.Unmarshal(params, &m); err != nil {
				t.Errorf("unmarshal params: %v", err)
				return nil
			}
			data, err := base64.StdEncoding.DecodeString(m["dataBase64"])
			if err != nil {
				t.Errorf("decode payload: %v", err)
				return nil
			}
			mu.Lock()
			got = append(got, string(data))
			park := first
			first = false
			mu.Unlock()

			enteredCh <- string(data)
			if park {
				<-releaseCh
			}
			return nil
		},
	}
	return b, enteredCh, func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), got...)
		}, func() {
			once.Do(func() { close(releaseCh) })
		}
}

// writeBigAsync performs a full-batch Write off the test goroutine and
// requires it to return promptly. Running it asynchronously means an
// implementation that blocks the write on an in-flight send fails the test
// rather than deadlocking the whole test binary.
func writeBigAsync(t *testing.T, b *pluginTerminalBridge, big []byte) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- b.Write(big) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Write(big): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Write of a full batch blocked while a send was in flight; it must decline the send slot and return")
	}
}

// TestPluginTerminalBridge_NoSecondBatchTakenWhileSendInFlight is the
// regression test for the reordering defect left by 11fe484.
//
// Under 11fe484, sendMu serialized sends but batches were detached from the
// buffer by any goroutine at any time. Two batches could therefore be taken in
// one order and then race for sendMu in the other, corrupting keystroke order.
// The fix takes a batch only when the send slot has already been claimed with
// TryLock, so at most one batch is ever in flight.
//
// Determinism: the reordering window itself is a preemption between two
// adjacent non-blocking statements and cannot be forced from a test. What this
// test drives deterministically is the *mechanism* that closes it — a full
// batch arriving while a send is provably in flight must NOT be detached. The
// test blocks until notify confirms the first send is inside the RPC (which,
// because notify only runs while sendMu is held, is structural proof the slot
// is taken), then writes a full batch.
//
// This fails on 11fe484 two ways: that Write would detach the batch (leaving
// the buffer empty) and would then block on sendMu for the whole parked send.
func TestPluginTerminalBridge_NoSecondBatchTakenWhileSendInFlight(t *testing.T) {
	b, entered, delivered, release := parkedBridge(t)
	defer release()

	if err := b.Write([]byte("first")); err != nil {
		t.Fatalf("Write(first): %v", err)
	}
	select {
	case got := <-entered:
		if got != "first" {
			t.Fatalf("first delivered payload = %q, want %q", got, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first batch to reach notify")
	}
	// sendMu is now provably held by the parked first send.

	big := make([]byte, terminalBatchMaxBytes)
	for i := range big {
		big[i] = 'x'
	}

	writeBigAsync(t, b, big)

	// The bytes must still be buffered: no second batch may be in flight.
	b.mu.Lock()
	buffered := len(b.buf)
	timerArmed := b.timer != nil
	b.mu.Unlock()
	if buffered != len(big) {
		t.Fatalf("buffered bytes = %d, want %d (batch must not be detached while a send is in flight)", buffered, len(big))
	}
	if !timerArmed {
		t.Fatal("declined flush left no armed timer; buffered bytes would be stranded")
	}

	release()

	select {
	case got := <-entered:
		if got != string(big) {
			t.Fatalf("second delivered payload = %q..., want the full batch", got[:min(8, len(got))])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the buffered batch to be delivered")
	}

	order := delivered()
	if len(order) != 2 || order[0] != "first" || order[1] != string(big) {
		t.Fatalf("delivery order wrong: got %d payloads, first=%q", len(order), order[0])
	}
}

// TestPluginTerminalBridge_DeclinedFlushIsNotStranded verifies the risk the
// TryLock shape introduces: bytes that could not claim the send slot must
// still be delivered without any further input from the user.
func TestPluginTerminalBridge_DeclinedFlushIsNotStranded(t *testing.T) {
	b, entered, _, release := parkedBridge(t)
	defer release()

	if err := b.Write([]byte("first")); err != nil {
		t.Fatalf("Write(first): %v", err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first batch to reach notify")
	}

	big := make([]byte, terminalBatchMaxBytes)
	for i := range big {
		big[i] = 'y'
	}
	// In a goroutine so an implementation that blocks here fails the test
	// instead of deadlocking it.
	writeBigAsync(t, b, big)

	// No further Write, Resize or Close: only the armed timer can deliver it.
	release()
	select {
	case got := <-entered:
		if got != string(big) {
			t.Fatalf("delivered %d bytes, want the %d-byte buffered batch", len(got), len(big))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("buffered batch was stranded: never delivered without further input")
	}
}

// TestPluginTerminalBridge_CloseFlushesBatchParkedBehindSend verifies Close
// does not drop a final batch that could not claim the send slot.
func TestPluginTerminalBridge_CloseFlushesBatchParkedBehindSend(t *testing.T) {
	b, entered, _, release := parkedBridge(t)
	defer release()

	if err := b.Write([]byte("first")); err != nil {
		t.Fatalf("Write(first): %v", err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first batch to reach notify")
	}

	big := make([]byte, terminalBatchMaxBytes)
	for i := range big {
		big[i] = 'z'
	}
	writeBigAsync(t, b, big)

	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()

	release()

	select {
	case got := <-entered:
		if len(got) != len(big) {
			t.Fatalf("delivered %d bytes, want the %d-byte final batch", len(got), len(big))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close lost the final batch")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}
