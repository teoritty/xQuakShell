package ipc_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"xquakshell/internal/infra/plugin/ipc"
)

func TestCodecRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	codec := ipc.NewCodec(&buf)

	params, _ := ipc.EncodeParams(map[string]string{"ping": "pong"})
	if err := codec.WriteMessage(ipc.NewRequest(1, "ping", params)); err != nil {
		t.Fatal(err)
	}

	msg, err := ipc.ReadMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Method != "ping" || msg.ID == nil || msg.ID.Key() != "1" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestRPCIDAcceptsStringAndNumberRejectsOthers(t *testing.T) {
	ok := []string{`"xqs-vnc-1"`, `1`, `-7`, `"1"`}
	for _, in := range ok {
		var id ipc.RPCID
		if err := json.Unmarshal([]byte(in), &id); err != nil {
			t.Errorf("Unmarshal(%s) rejected a valid id: %v", in, err)
			continue
		}
		out, err := json.Marshal(id)
		if err != nil {
			t.Errorf("Marshal(%s): %v", in, err)
			continue
		}
		if string(out) != in {
			t.Errorf("round-trip %s -> %s: id must echo verbatim", in, out)
		}
	}
	// A numeric 1 and a string "1" are distinct ids and must not share a key.
	var num, str ipc.RPCID
	_ = json.Unmarshal([]byte(`1`), &num)
	_ = json.Unmarshal([]byte(`"1"`), &str)
	if num.Key() == str.Key() {
		t.Errorf("numeric 1 and string \"1\" collided on key %q", num.Key())
	}
	for _, bad := range []string{`null`, `{}`, `[1]`, `true`, `1.5e2x`} {
		var id ipc.RPCID
		if err := json.Unmarshal([]byte(bad), &id); err == nil {
			t.Errorf("Unmarshal(%s) accepted an invalid id", bad)
		}
	}
}

func TestCodecWriteRejectsOversizedFrame(t *testing.T) {
	var buf bytes.Buffer
	codec := ipc.NewCodec(&buf)

	large := make([]byte, 300<<10)
	params, _ := ipc.EncodeParams(map[string]string{"data": string(large)})
	err := codec.WriteMessage(ipc.NewRequest(1, "fs.read", params))
	if err == nil {
		t.Fatal("expected oversize frame write to fail")
	}
}

func TestConnCall(t *testing.T) {
	coreReader, pluginWrite := io.Pipe()
	pluginReader, coreWrite := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pluginWrite.Close()
		serveEchoPlugin(pluginReader, pluginWrite)
	}()

	conn := ipc.NewConn(coreReader, coreWrite, nil, nil, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := conn.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]string
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["pong"] != "ok" {
		t.Fatalf("unexpected result: %v", payload)
	}

	_ = coreWrite.Close()
	_ = pluginWrite.Close()
	conn.Close()
	wg.Wait()
}

func serveEchoPlugin(r io.Reader, w io.Writer) {
	reader := bufio.NewReader(r)
	codec := ipc.NewCodec(w)
	for {
		msg, err := ipc.ReadMessage(reader)
		if err != nil {
			return
		}
		if msg.ID == nil {
			continue
		}
		result, _ := ipc.EncodeResult(map[string]string{"pong": "ok"})
		_ = codec.WriteMessage(ipc.NewResponse(*msg.ID, result))
	}
}
