package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestConnHandlesConcurrentPluginRequests(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	hostInR, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInR.Close()
	})

	var readStarted atomic.Bool
	var writeHandled atomic.Bool

	handler := func(ctx context.Context, method string, _ json.RawMessage) (json.RawMessage, *RPCError) {
		switch method {
		case "net.read":
			readStarted.Store(true)
			select {
			case <-ctx.Done():
				raw, _ := json.Marshal(map[string]any{"contentBase64": "", "eof": false})
				return raw, nil
			case <-time.After(500 * time.Millisecond):
				raw, _ := json.Marshal(map[string]any{"contentBase64": "", "eof": false})
				return raw, nil
			}
		case "net.write":
			writeHandled.Store(true)
			raw, _ := json.Marshal(map[string]bool{"ok": true})
			return raw, nil
		default:
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
	}

	conn := NewConn(pluginOutR, hostInW, nil, handler, 0)

	go func() {
		r := bufio.NewReader(hostInR)
		for {
			if _, err := ReadMessage(r); err != nil {
				return
			}
		}
	}()

	pluginEnc := NewCodec(pluginOutW)
	if err := pluginEnc.WriteMessage(NewRequest(1, "net.read", nil)); err != nil {
		t.Fatalf("write net.read: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if readStarted.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !readStarted.Load() {
		t.Fatal("net.read handler did not start")
	}

	if err := pluginEnc.WriteMessage(NewRequest(2, "net.write", nil)); err != nil {
		t.Fatalf("write net.write: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if writeHandled.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !writeHandled.Load() {
		t.Fatal("net.write was not handled while net.read was in progress")
	}

	_ = pluginOutW.Close()
	conn.Close()
}

// TestConnDispatchesStringIDRequest reproduces the xqs-plugin-vnc failure: a
// plugin whose JSON-RPC ids are strings ("xqs-vnc-N", valid per JSON-RPC 2.0)
// must have its requests dispatched and answered, with the id echoed back
// verbatim. Before RPCID the host decoded id as *int64, so a string id failed
// to parse and the whole frame was dropped, hanging the plugin's channel.open.
func TestConnDispatchesStringIDRequest(t *testing.T) {
	pluginOutR, pluginOutW := io.Pipe()
	hostInR, hostInW := io.Pipe()
	t.Cleanup(func() {
		_ = pluginOutW.Close()
		_ = hostInR.Close()
	})

	var handled atomic.Bool
	handler := func(_ context.Context, method string, _ json.RawMessage) (json.RawMessage, *RPCError) {
		if method == "channel.open" {
			handled.Store(true)
		}
		raw, _ := json.Marshal(map[string]bool{"ok": true})
		return raw, nil
	}

	conn := NewConn(pluginOutR, hostInW, nil, handler, 0)
	defer conn.Close()

	fw := NewFrameWriter(pluginOutW)
	req := []byte(`{"jsonrpc":"2.0","id":"xqs-vnc-1","method":"channel.open","params":{}}`)
	go func() {
		_ = fw.Write(domainplugin.FrameKindJSONRPC, 0, req)
	}()

	resp, err := ReadMessage(bufio.NewReader(hostInR))
	if err != nil {
		t.Fatalf("read host response: %v", err)
	}
	if !handled.Load() {
		t.Fatal("channel.open with a string id was never dispatched to onRequest")
	}
	if resp.ID == nil || resp.ID.Key() != `"xqs-vnc-1"` {
		t.Fatalf("response id = %v, want the request's string id echoed verbatim", resp.ID)
	}
}
