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

	"ssh-client/internal/infra/plugin/ipc"
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
	if msg.Method != "ping" || msg.ID == nil || *msg.ID != 1 {
		t.Fatalf("unexpected message: %+v", msg)
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

	conn := ipc.NewConn(coreReader, coreWrite, nil, nil)

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
