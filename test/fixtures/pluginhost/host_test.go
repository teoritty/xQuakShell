package pluginhost

import (
	"bufio"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// TestHostAnswersCoreRequestWhoseIDCollidesWithAnInFlightCall covers the flake that took
// TestUIEndToEndTerminalSurfaceReceivesInput down in CI.
//
// Both peers number their own JSON-RPC requests from 1, so the id spaces overlap. The fixture's
// read loop matched every id against its own pending calls first, so a core request that happened
// to share a number with a call the fixture had in flight — surface.write, say, still unanswered
// while the test asked for fixture.state — was posted to that call's reply channel and never
// handled. The core then waited out its full timeout for a reply nobody was going to write.
//
// The plugin fixtures share this loop, so the collision was reachable from any test that talks to
// a fixture while the fixture is mid-call. It is timing-dependent on the wire and deterministic
// here.
func TestHostAnswersCoreRequestWhoseIDCollidesWithAnInFlightCall(t *testing.T) {
	coreToPluginR, coreToPluginW := io.Pipe()
	pluginToCoreR, pluginToCoreW := io.Pipe()
	t.Cleanup(func() {
		_ = coreToPluginW.Close()
		_ = pluginToCoreR.Close()
	})

	host := NewHostFromStreams(coreToPluginR, pluginToCoreW)
	host.Register("fixture.state", func(json.RawMessage) (any, error) {
		return map[string]string{"state": "ready"}, nil
	})

	// Everything the fixture writes, drained by one goroutine: a blocking read would hang the test
	// on the very failure it exists to report.
	fromPlugin := make(chan Message, 4)
	go func() {
		r := bufio.NewReader(pluginToCoreR)
		for {
			msg, err := readMessage(r)
			if err != nil {
				close(fromPlugin)
				return
			}
			fromPlugin <- msg
		}
	}()

	// The fixture calls the core and is still waiting on the answer.
	type callResult struct {
		raw json.RawMessage
		err error
	}
	calls := make(chan callResult, 1)
	go func() {
		raw, err := host.CallCore("surface.write", map[string]string{"surfaceId": "s-1"})
		calls <- callResult{raw, err}
	}()

	pluginRequest := expectMessage(t, fromPlugin, "the fixture's own request")
	if pluginRequest.ID == nil {
		t.Fatal("the fixture's request carried no id")
	}

	// The core asks a question under the same number. It is a request, not the answer to anything.
	coreWriter := newJSONWriter(coreToPluginW)
	go func() {
		_ = coreWriter.WriteMessage(Message{
			JSONRPC: jsonRPCVersion,
			ID:      pluginRequest.ID,
			Method:  "fixture.state",
		})
	}()

	reply := expectMessage(t, fromPlugin, "the fixture's reply to fixture.state")
	if reply.Method != "" {
		t.Fatalf("expected a reply, got a request for %q", reply.Method)
	}
	if reply.ID == nil || *reply.ID != *pluginRequest.ID {
		t.Fatalf("reply id = %v, want %v echoed", reply.ID, pluginRequest.ID)
	}
	var state map[string]string
	if err := json.Unmarshal(reply.Result, &state); err != nil {
		t.Fatalf("decode reply: %v (result=%s)", err, reply.Result)
	}
	if state["state"] != "ready" {
		t.Fatalf("state = %v", state)
	}

	select {
	case got := <-calls:
		t.Fatalf("CallCore returned early (raw=%s err=%v) — it took the core's request for its answer", got.raw, got.err)
	case <-time.After(100 * time.Millisecond):
	}

	// The core's real answer, under the same number, still resolves the fixture's call.
	answer, _ := json.Marshal(map[string]bool{"ok": true})
	go func() {
		_ = coreWriter.WriteMessage(Message{JSONRPC: jsonRPCVersion, ID: pluginRequest.ID, Result: answer})
	}()

	select {
	case got := <-calls:
		if got.err != nil {
			t.Fatalf("CallCore: %v", got.err)
		}
		if string(got.raw) != string(answer) {
			t.Fatalf("CallCore result = %s, want %s", got.raw, answer)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CallCore never saw its response")
	}
}

func expectMessage(t *testing.T, msgs <-chan Message, what string) Message {
	t.Helper()
	select {
	case msg, ok := <-msgs:
		if !ok {
			t.Fatalf("the stream closed before %s", what)
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return Message{}
	}
}
