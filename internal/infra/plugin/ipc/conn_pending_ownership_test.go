package ipc

import (
	"encoding/json"
	"io"
	"testing"
	"time"
)

// TestFailReadLoopDoesNotDeadlockOnAbandonedPendingEntry reproduces the deadlock where
// failReadLoop sends into a size-1 pending channel while holding c.mu, and nobody is left
// to drain it.
//
// The real trigger is a data race in Call: its select between <-ctx.Done() and <-ch can
// pick ctx.Done() *after* the dispatcher has already filled ch's buffer but *before* Call's
// deferred cleanup removes the entry from c.pending. Forcing that exact scheduler race from
// a test would make the test itself racy (a coin flip on which case a ready select picks),
// so this test does not use Call or a real context at all. Instead it manufactures the
// state Call leaves behind when that race goes the bad way — an entry in c.pending whose
// channel nobody will ever read again — directly and deterministically, then drives the
// real dispatcher (readLoop) and real failReadLoop (via a real read error from closing the
// plugin's write end) over that state. That's the code this task actually changes; how
// Call happens to abandon the entry is incidental to it.
//
// Determinism note: after writing responses for id=1 and id=2, the test blocks on
// receiving id=2's response. readLoop dispatches frames one at a time in a single
// goroutine, so by the time id=2's send is observable, id=1's send (the one that matters)
// has already completed — no sleep required.
func TestFailReadLoopDoesNotDeadlockOnAbandonedPendingEntry(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	_, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInW.Close()
	})

	conn := NewConn(pluginOutR, hostInW, nil, nil, 0)

	// Simulate two Calls that registered their reply channel and then abandoned it without
	// ever reading a response — exactly the state left behind when ctx.Done() wins Call's
	// select. Buffer size 1 matches production (conn.go: make(chan messageResult, 1)).
	key1 := int64ID(1).Key()
	ch1 := make(chan messageResult, 1)
	key2 := int64ID(2).Key()
	ch2 := make(chan messageResult, 1)
	conn.mu.Lock()
	conn.pending[key1] = ch1
	conn.pending[key2] = ch2
	conn.mu.Unlock()

	pluginEnc := NewCodec(pluginOutW)
	go func() {
		_ = pluginEnc.WriteMessage(NewResponse(int64ID(1), json.RawMessage(`{}`)))
		_ = pluginEnc.WriteMessage(NewResponse(int64ID(2), json.RawMessage(`{}`)))
	}()

	// Block until the dispatcher has processed id=2's frame. Since readLoop handles frames
	// strictly in order on a single goroutine, id=1's dispatch (the send into ch1 that fills
	// its buffer) is guaranteed to have already completed by the time this unblocks.
	select {
	case <-ch2:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher never delivered the response for id=2; readLoop appears stuck")
	}

	// ch1 is now full and permanently unread (the simulated Call already gave up). Closing
	// the plugin's write end (not calling Conn.Close()) forces a real read error in the
	// host's readLoop, which drives it into a real failReadLoop call against this state.
	//
	// This has to be the plugin-side close, not Conn.Close(): Close's first act is closing
	// c.closeCh, and failReadLoop returns early if closeCh is already closed when it runs.
	// Triggering the error through Conn.Close() would race that early-return against the
	// read error and could mask the bug — which is exactly what happened when this test was
	// first written against Conn.Close() as the trigger: it passed against the unfixed code.
	_ = pluginOutW.Close()

	// Wait on c.wg directly rather than calling Conn.Close() yet. c.wg.Wait() needs no lock,
	// so — unlike ReadError() or Close() — it can't itself get stuck behind the mutex
	// failReadLoop holds while deadlocked; it simply never returns, which is exactly the
	// symptom under test. readLoop's sole goroutine calls wg.Done() only after failReadLoop
	// returns, so this blocks forever pre-fix and returns promptly post-fix.
	wgDone := make(chan struct{})
	go func() {
		conn.wg.Wait()
		close(wgDone)
	}()

	select {
	case <-wgDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ipc.readLoop never exited: failReadLoop deadlocked sending into an abandoned pending channel while holding c.mu")
	}

	// Now that the read loop has genuinely exited, Conn.Close() (the public, documented
	// symptom from the task brief) must also return promptly.
	done := make(chan struct{})
	go func() {
		conn.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Conn.Close blocked even after ipc.readLoop exited")
	}
}
