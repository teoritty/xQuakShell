package plugin_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/infra/plugin/capability"
	"ssh-client/internal/infra/plugin/ipc"
)

func TestGateDeniesFSWithoutCapability(t *testing.T) {
	gate := capability.NewGate(domainplugin.Manifest{})
	server := ipc.NewHostServer(ipc.HostServerConfig{
		PluginID: "test",
		Gate:     gate,
		FS:       mustFSProxy(t, nil, t.TempDir()),
	})

	_, rpcErr := server.HandleRequest(context.Background(), "fs.read", mustJSON(map[string]string{
		"path": "note.txt",
	}))
	if rpcErr == nil || rpcErr.Code != -32001 {
		t.Fatalf("expected capability denied, got %#v", rpcErr)
	}
}

func TestFSProxyAllowsPluginDataOnly(t *testing.T) {
	dataDir := t.TempDir()
	vaultFile := filepath.Join(t.TempDir(), "vault.age")

	fsCaps := &domainplugin.FSCaps{Read: []string{"${pluginData}"}}
	fs := mustFSProxy(t, fsCaps, dataDir)

	allowedFile := filepath.Join(dataDir, "ok.txt")
	if err := os.WriteFile(allowedFile, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	params := mustJSON(map[string]string{"path": "ok.txt"})
	if _, err := fs.Handle("fs.read", params); err != nil {
		t.Fatalf("expected read ok: %v", err)
	}

	outside := mustJSON(map[string]string{"path": vaultFile})
	if _, err := fs.Handle("fs.read", outside); err == nil {
		t.Fatal("expected vault path denied")
	}
}

func TestValidateManifestRejectsWildcardNetwork(t *testing.T) {
	m := domainplugin.Manifest{
		ID: "com.test.wild", Name: "x", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Network: &domainplugin.NetworkCaps{Outbound: []string{"*"}},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected wildcard network to be rejected")
	}
}

func TestValidateManifestRejectsHostWildcard(t *testing.T) {
	m := domainplugin.Manifest{
		ID: "com.test.hostwild", Name: "x", Version: "1",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Network: &domainplugin.NetworkCaps{Outbound: []string{"tcp:*:443"}},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected tcp:*:443 to be rejected")
	}
}

func TestFSProxyRejectsOversizeFile(t *testing.T) {
	dataDir := t.TempDir()
	fsCaps := &domainplugin.FSCaps{Read: []string{"${pluginData}"}}
	fs := mustFSProxy(t, fsCaps, dataDir)

	large := filepath.Join(dataDir, "big.bin")
	if err := os.WriteFile(large, make([]byte, domainplugin.MaxFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	params := mustJSON(map[string]string{"path": "big.bin"})
	if _, err := fs.Handle("fs.read", params); err == nil {
		t.Fatal("expected oversize file denied")
	}
}

func TestFSProxyChunkedRead(t *testing.T) {
	dataDir := t.TempDir()
	fsCaps := &domainplugin.FSCaps{Read: []string{"${pluginData}"}}
	fs := mustFSProxy(t, fsCaps, dataDir)

	payload := make([]byte, capability.MaxReadBytes+128)
	for i := range payload {
		payload[i] = byte('a' + (i % 26))
	}
	if err := os.WriteFile(filepath.Join(dataDir, "chunked.bin"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	raw, err := fs.Handle("fs.read", mustJSON(map[string]any{"path": "chunked.bin", "offset": 0}))
	if err != nil {
		t.Fatalf("first chunk: %v", err)
	}
	var first struct {
		ContentBase64 string `json:"contentBase64"`
		EOF           bool   `json:"eof"`
		TotalSize     int64  `json:"totalSize"`
	}
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatal(err)
	}
	if first.EOF {
		t.Fatal("expected more chunks")
	}
	if first.TotalSize != int64(len(payload)) {
		t.Fatalf("totalSize=%d want %d", first.TotalSize, len(payload))
	}

	raw, err = fs.Handle("fs.read", mustJSON(map[string]any{
		"path":   "chunked.bin",
		"offset": capability.MaxReadBytes,
	}))
	if err != nil {
		t.Fatalf("second chunk: %v", err)
	}
	var second struct {
		EOF bool `json:"eof"`
	}
	if err := json.Unmarshal(raw, &second); err != nil {
		t.Fatal(err)
	}
	if !second.EOF {
		t.Fatal("expected EOF on final chunk")
	}
}

func TestReadMessageRejectsOversizeFrame(t *testing.T) {
	var buf bytes.Buffer
	oversized := make([]byte, domainplugin.MaxFrameBytes+10)
	for i := range oversized {
		oversized[i] = 'a'
	}
	buf.Write(oversized)
	buf.WriteByte('\n')

	_, err := ipc.ReadMessage(bufio.NewReader(&buf))
	if err != ipc.ErrFrameTooLarge {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
}

func mustFSProxy(t *testing.T, caps *domainplugin.FSCaps, dataDir string) *capability.FSProxy {
	t.Helper()
	fs, err := capability.NewFSProxy(caps, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
