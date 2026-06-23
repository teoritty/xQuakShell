package plugin_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/infra/plugin/ipc"
)

func BenchmarkPluginIPCPingPong(b *testing.B) {
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
	defer conn.Close()
	defer coreWrite.Close()
	defer pluginWrite.Close()

	ctx := context.Background()
	params, _ := json.Marshal(map[string]string{"hello": "world"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := conn.Call(ctx, "ping", params); err != nil {
			b.Fatal(err)
		}
	}
	wg.Wait()
}

func TestPluginIPCPingPongSmoke(t *testing.T) {
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

	_ = coreWrite.Close()
	_ = pluginWrite.Close()
	conn.Close()
	wg.Wait()

	if string(result) != `{"pong":"ok"}` {
		t.Fatalf("unexpected result %s", result)
	}
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
