package plugin_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/domain"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
	"ssh-client/internal/usecase"
)

type sessionCapture struct {
	mu     sync.Mutex
	states []string
	output []byte
}

func (s *sessionCapture) HandlePluginUpdateState(_ string, _ string, state, _ string) error {
	s.mu.Lock()
	s.states = append(s.states, state)
	s.mu.Unlock()
	return nil
}

func (s *sessionCapture) HandlePluginWriteTerminal(_ string, _ string, data []byte) error {
	s.mu.Lock()
	s.output = append(s.output, data...)
	s.mu.Unlock()
	return nil
}

func (s *sessionCapture) snapshot() (states []string, output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.states...), append([]byte(nil), s.output...)
}

func TestDemoTerminalSessionConnect(t *testing.T) {
	pluginDir := buildDemoTerminalPlugin(t)

	inbound := usecase.NewPluginSessionInbound()
	capture := &sessionCapture{}
	inbound.SetHandler(capture)

	registry := usecase.NewPluginRegistry()
	auth := usecase.NewPluginSessionAuthorizer(registry)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionRPC:        usecase.NewPluginSessionRPCHandlerFactory(inbound, auth),
		SessionAuthorizer: auth,
	})
	manager := newTestPluginManager(t, registry, host)

	discovery := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := manager.DiscoverPlugins(discovery.Discover); err != nil {
		t.Fatal(err)
	}

	bridge := usecase.NewPluginSessionBridge(manager)
	conn := &domain.Connection{
		ID:       "conn-1",
		Host:     "example.com",
		Port:     9999,
		Protocol: "demo-terminal",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const (
		pluginID  = "com.xquakshell.demo-terminal"
		sessionID = "sess-1"
	)

	if err := bridge.Connect(ctx, pluginID, sessionID, conn); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, 10*time.Second, func() bool {
		states, output := capture.snapshot()
		return containsString(states, "ready") && bytes.Contains(output, []byte("Demo Terminal"))
	})

	params, err := json.Marshal(map[string]string{
		"sessionId":  sessionID,
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.NotifyForSession(ctx, pluginID, sessionID, "session.writeInput", params); err != nil {
		t.Fatal(err)
	}

	waitUntil(t, 5*time.Second, func() bool {
		_, output := capture.snapshot()
		return bytes.Contains(output, []byte("hello"))
	})

	bridge.Disconnect(ctx, pluginID, sessionID)
	manager.StopAll(ctx)
}

func buildDemoTerminalPlugin(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	pluginSrc := filepath.Join(root, "plugins", "demo-terminal")
	outDir := t.TempDir()

	binName := "demo-terminal"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(outDir, binName)

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-trimpath", "-o", binPath, "./plugins/demo-terminal")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build demo-terminal: %v\n%s", err, out)
	}

	manifestSrc := filepath.Join(pluginSrc, "plugin.json")
	manifestData, err := os.ReadFile(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	pluginInstallDir := filepath.Join(outDir, "demo-terminal")
	if err := os.MkdirAll(pluginInstallDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginInstallDir, "plugin.json"), manifestData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginInstallDir, binName), readFile(t, binPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(pluginInstallDir); err != nil {
		t.Fatal(err)
	}
	return pluginInstallDir
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}
