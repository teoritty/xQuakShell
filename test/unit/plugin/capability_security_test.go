package plugin_test

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
)

func TestNetProxyRejectsPortOnlyPattern(t *testing.T) {
	proxy := capability.NewNetProxy("test", &domainplugin.NetworkCaps{
		Outbound: []string{"tcp:443"},
	})
	_, err := proxy.Dial(mustJSON(map[string]any{
		"host": "evil.example.com",
		"port": 443,
	}))
	if err == nil {
		t.Fatal("expected dial denied for port-only pattern")
	}
}

func TestNetProxyAllowsExplicitHostPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	proxy := capability.NewNetProxy("test", &domainplugin.NetworkCaps{
		Outbound: []string{fmt.Sprintf("tcp:127.0.0.1:%d", port)},
	})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("ok"))
	}()

	raw, err := proxy.Dial(mustJSON(map[string]any{
		"host": "127.0.0.1",
		"port": port,
	}))
	if err != nil {
		t.Fatalf("expected dial ok, got %v", err)
	}
	var dialOut struct {
		HandleID string `json:"handleId"`
	}
	if err := json.Unmarshal(raw, &dialOut); err != nil {
		t.Fatal(err)
	}
	if dialOut.HandleID == "" {
		t.Fatal("expected handleId")
	}
	readRaw, err := proxy.Read(mustJSON(map[string]any{
		"handleId": dialOut.HandleID,
	}))
	if err != nil {
		t.Fatalf("expected read ok: %v", err)
	}
	var readOut struct {
		ContentBase64 string `json:"contentBase64"`
	}
	if err := json.Unmarshal(readRaw, &readOut); err != nil {
		t.Fatal(err)
	}
	if readOut.ContentBase64 == "" {
		t.Fatal("expected payload")
	}
	if _, err := proxy.Close(mustJSON(map[string]string{"handleId": dialOut.HandleID})); err != nil {
		t.Fatalf("expected close ok: %v", err)
	}
}

func TestFSProxyWriteCreatesNewFile(t *testing.T) {
	dataDir := t.TempDir()
	fsCaps := &domainplugin.FSCaps{Write: []string{"${pluginData}"}}
	fs := mustFSProxy(t, fsCaps, dataDir)

	params := mustJSON(map[string]string{
		"path":          "new.txt",
		"contentBase64": "aGVsbG8=",
	})
	if _, err := fs.Handle("fs.write", params); err != nil {
		t.Fatalf("expected write ok: %v", err)
	}
}
