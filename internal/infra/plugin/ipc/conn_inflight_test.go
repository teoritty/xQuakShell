package ipc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// parkingConn is a Conn whose handler blocks until the test releases it, which is what a proxy
// waiting on a syscall looks like from the connection's side.
type parkingConn struct {
	conn        *Conn
	out         *syncBuffer
	entered     chan struct{}
	release     chan struct{}
	peak        atomic.Int32
	active      atomic.Int32
	releaseOnce *sync.Once
}

func newParkingConn(t *testing.T) *parkingConn {
	t.Helper()
	p := &parkingConn{
		out:     &syncBuffer{},
		entered: make(chan struct{}, 4096),
		release: make(chan struct{}),
	}
	handler := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, *RPCError) {
		n := p.active.Add(1)
		for {
			peak := p.peak.Load()
			if n <= peak || p.peak.CompareAndSwap(peak, n) {
				break
			}
		}
		p.entered <- struct{}{}
		<-p.release
		p.active.Add(-1)
		return json.RawMessage(`{"ok":true}`), nil
	}
	// An empty reader: this test drives dispatch directly, so the read loop has nothing to do.
	p.conn = NewConn(strings.NewReader(""), p.out, nil, handler, 0)
	var once sync.Once
	t.Cleanup(func() {
		once.Do(func() { close(p.release) })
		p.conn.Close()
	})
	p.releaseOnce = &once
	return p
}

// releaseAll unparks every handler exactly once, whether the test or the cleanup gets there first.
func (p *parkingConn) releaseAll() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func inflightRPCID(t *testing.T, n int) RPCID {
	t.Helper()
	var id RPCID
	if err := json.Unmarshal([]byte(inflightItoa(n)), &id); err != nil {
		t.Fatalf("build rpc id %d: %v", n, err)
	}
	return id
}

func inflightItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// fillEverySlot dispatches exactly MaxConcurrentPluginRequests requests and waits for each to be
// inside the handler, so every slot is genuinely held rather than merely dispatched.
func (p *parkingConn) fillEverySlot(t *testing.T) {
	t.Helper()
	for i := 1; i <= domainplugin.MaxConcurrentPluginRequests; i++ {
		p.conn.dispatchInboundRequest(inflightRPCID(t, i), "ping", json.RawMessage(`{}`))
	}
	for i := 0; i < domainplugin.MaxConcurrentPluginRequests; i++ {
		select {
		case <-p.entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d requests reached the handler", i, domainplugin.MaxConcurrentPluginRequests)
		}
	}
}

// The hole this closes: every inbound request was dispatched on its own host goroutine with no cap,
// and only log.write was rate limited. A plugin could pump requests at line rate and the host would
// grow a goroutine for each. The 30s context deadline bounds a well-behaved handler; a proxy parked
// in a syscall does not observe its context. The plugin process is memory-capped by its job object
// or rlimit - the host is not, which is what makes this the host's problem.
func TestInboundRequestsAreCappedAtTheConcurrencyLimit(t *testing.T) {
	p := newParkingConn(t)
	p.fillEverySlot(t)

	p.conn.dispatchInboundRequest(inflightRPCID(t, 9001), "ping", json.RawMessage(`{}`))

	// The over-limit request must be answered, not dispatched: nothing new enters the handler.
	select {
	case <-p.entered:
		t.Fatal("a request past the concurrency cap still reached the handler")
	case <-time.After(200 * time.Millisecond):
	}
	if reply := p.out.String(); !strings.Contains(reply, "rate limited") {
		t.Fatalf("reply to the over-limit request = %q, want a rate-limited error", reply)
	}
	if got := p.peak.Load(); int(got) > domainplugin.MaxConcurrentPluginRequests {
		t.Errorf("peak concurrent handlers = %d, above the cap of %d", got, domainplugin.MaxConcurrentPluginRequests)
	}
}

// The refusal must be non-blocking. Waiting for a slot would stall the read loop, and that loop
// also delivers responses to the host's OWN outbound calls - so a plugin could deadlock the
// connection by filling the semaphore and then answering nothing.
func TestARefusalDoesNotBlockTheCaller(t *testing.T) {
	p := newParkingConn(t)
	p.fillEverySlot(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.conn.dispatchInboundRequest(inflightRPCID(t, 9002), "ping", json.RawMessage(`{}`))
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch blocked while the cap was full; the read loop would stall here and the " +
			"connection would deadlock")
	}
}

// The slot must come back when a handler finishes, or the cap is a one-shot budget and the plugin
// stops being served after MaxConcurrentPluginRequests requests for the life of the process.
func TestSlotsAreReturnedWhenHandlersComplete(t *testing.T) {
	p := newParkingConn(t)
	p.fillEverySlot(t)
	p.releaseAll()

	// Wait for the parked handlers to drain their slots.
	deadline := time.After(5 * time.Second)
	for p.active.Load() > 0 {
		select {
		case <-deadline:
			t.Fatalf("%d handlers never returned", p.active.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	// A fresh request must be accepted. The handler no longer parks, so reaching it is the proof.
	p.conn.dispatchInboundRequest(inflightRPCID(t, 9003), "ping", json.RawMessage(`{}`))
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("a request after the burst drained was still refused; slots are never released")
	}
}

// A plugin under the cap must be unaffected: the limit is there to bound a flood, not to make
// ordinary parallel work fail.
func TestRequestsBelowTheCapAreAllDispatched(t *testing.T) {
	p := newParkingConn(t)

	const burst = 8
	for i := 1; i <= burst; i++ {
		p.conn.dispatchInboundRequest(inflightRPCID(t, i), "ping", json.RawMessage(`{}`))
	}
	for i := 0; i < burst; i++ {
		select {
		case <-p.entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d ordinary concurrent requests were dispatched", i, burst)
		}
	}
	if reply := p.out.String(); strings.Contains(reply, "rate limited") {
		t.Errorf("an ordinary burst of %d was rate limited: %s", burst, reply)
	}
}
