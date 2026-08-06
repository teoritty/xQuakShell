package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestConnDispatchesPluginRequestWhoseIDCollidesWithAnInFlightCall pins the rule JSON-RPC 2.0
// gives a bidirectional connection: each peer numbers its own requests, so the two id spaces
// overlap by construction. The host's call #3 and the plugin's request #3 are different messages
// that happen to share a number, and only `method` tells them apart.
//
// Routing by id first made an inbound request look like the answer to whatever the host had
// outstanding under that number: the plugin's request was swallowed into the pending channel and
// never dispatched, so the plugin waited out its own timeout for a reply nobody was writing —
// while the host's Call took the request as its answer and returned a nil result.
func TestConnDispatchesPluginRequestWhoseIDCollidesWithAnInFlightCall(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	hostInR, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInR.Close()
	})

	var dispatched atomic.Bool
	handler := func(_ context.Context, method string, _ json.RawMessage) (json.RawMessage, *RPCError) {
		if method == "discovery.publish" {
			dispatched.Store(true)
		}
		raw, _ := json.Marshal(map[string]bool{"ok": true})
		return raw, nil
	}

	conn := NewConn(pluginOutR, hostInW, nil, handler, 0)
	defer conn.Close()

	// Everything the host puts on the wire, drained by one goroutine: a blocking read here would
	// hang the test on exactly the failure it is meant to report.
	hostWrote := make(chan Message, 4)
	go func() {
		r := bufio.NewReader(hostInR)
		for {
			msg, err := ReadMessage(r)
			if err != nil {
				close(hostWrote)
				return
			}
			hostWrote <- msg
		}
	}()

	type callResult struct {
		raw json.RawMessage
		err error
	}
	hostReplies := make(chan callResult, 1)
	go func() {
		raw, err := conn.Call(context.Background(), "fixture.state", nil)
		hostReplies <- callResult{raw, err}
	}()

	hostRequest := expectHostMessage(t, hostWrote, "the host's own request")
	if hostRequest.ID == nil {
		t.Fatal("the host's request carried no id")
	}

	// The plugin sends a request of its own under the same id, as any plugin numbering from 1 will.
	pluginFrame := []byte(`{"jsonrpc":"2.0","id":` + hostRequest.ID.Key() + `,"method":"discovery.publish","params":{}}`)
	fw := NewFrameWriter(pluginOutW)
	go func() { _ = fw.Write(domainplugin.FrameKindJSONRPC, 0, pluginFrame) }()

	reply := expectHostMessage(t, hostWrote, "the host's reply to the plugin's request")
	if !dispatched.Load() {
		t.Fatal("the plugin's request was never dispatched — it was taken for a response")
	}
	if reply.ID == nil || reply.ID.Key() != hostRequest.ID.Key() {
		t.Fatalf("reply id = %v, want the plugin's request id echoed", reply.ID)
	}
	if reply.Result == nil {
		t.Fatalf("reply carried no result: %+v", reply)
	}

	select {
	case got := <-hostReplies:
		t.Fatalf("the host's Call returned early (raw=%s err=%v) — it took the plugin's request for its answer", got.raw, got.err)
	case <-time.After(100 * time.Millisecond):
	}

	// The plugin's real answer, under the same number, still resolves the host's call.
	pluginEnc := NewCodec(pluginOutW)
	answer, _ := json.Marshal(map[string]string{"state": "ready"})
	go func() { _ = pluginEnc.WriteMessage(NewResponse(*hostRequest.ID, answer)) }()

	select {
	case got := <-hostReplies:
		if got.err != nil {
			t.Fatalf("Call: %v", got.err)
		}
		if string(got.raw) != string(answer) {
			t.Fatalf("Call result = %s, want %s", got.raw, answer)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the host's Call never saw its response")
	}
}

func expectHostMessage(t *testing.T, msgs <-chan Message, what string) Message {
	t.Helper()
	select {
	case msg, ok := <-msgs:
		if !ok {
			t.Fatalf("the connection closed before %s", what)
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return Message{}
	}
}
