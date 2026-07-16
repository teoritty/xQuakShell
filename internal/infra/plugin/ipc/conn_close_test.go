package ipc

import (
	"io"
	"testing"
	"time"
)

// TestConnCloseReturnsWhileReadLoopIsParked covers the teardown path of a plugin that is hung:
// it never sends another frame and never closes its stdout. Close must still return, because
// managedProcess.closeResources calls it *before* killing the child — so a Close that blocks
// here makes the kill unreachable and wedges host teardown permanently.
func TestConnCloseReturnsWhileReadLoopIsParked(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	_, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInW.Close()
	})

	// Nothing is ever written to pluginOutW: the read loop parks in ReadFrame on the first
	// header byte, exactly like a wedged plugin's idle stdout.
	conn := NewConn(pluginOutR, hostInW, nil, nil)

	done := make(chan struct{})
	go func() {
		conn.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Conn.Close blocked on a parked read loop; a hung plugin would wedge host teardown")
	}
}

// TestConnCloseIsIdempotent guards the close-once path: closeResources and any crash-teardown
// path may both reach Close, and the second call must not panic on an already-closed pipe.
func TestConnCloseIsIdempotent(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	_, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInW.Close()
	})

	conn := NewConn(pluginOutR, hostInW, nil, nil)

	done := make(chan struct{})
	go func() {
		conn.Close()
		conn.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("second Conn.Close blocked")
	}
}
