package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
	"ssh-client/internal/usecase"
)

func newTestPluginManager(t *testing.T, registry *usecase.PluginRegistry, host domainplugin.ProcessHost) *usecase.PluginManager {
	t.Helper()
	return usecase.NewPluginManagerWithConfig(usecase.PluginManagerConfig{
		Registry:    registry,
		Host:        host,
		InstallRoot: t.TempDir(),
	})
}

func TestPluginManagerEchoPing(t *testing.T) {
	pluginDir := buildExampleEchoPlugin(t)

	registry := usecase.NewPluginRegistry()
	auth := usecase.NewPluginSessionAuthorizer(registry)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{
		DataRoot:          t.TempDir(),
		SessionAuthorizer: auth,
	})
	manager := newTestPluginManager(t, registry, host)

	discovery := infraplugin.NewDiscovery([]string{filepath.Dir(pluginDir)})
	if err := manager.DiscoverPlugins(discovery.Discover); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const pluginID = "com.xquakshell.example-echo"
	if err := manager.EnsureRunning(ctx, pluginID); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Ping(ctx, pluginID)
	if err != nil {
		t.Fatal(err)
	}
	if result["pong"] != "ok" {
		t.Fatalf("unexpected ping result: %v", result)
	}

	manager.StopAll(ctx)
}

func buildExampleEchoPlugin(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	pluginSrc := filepath.Join(root, "plugins", "example-echo")
	outDir := t.TempDir()

	binName := "example-echo"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(outDir, binName)

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-trimpath", "-o", binPath, "./plugins/example-echo")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build example-echo: %v\n%s", err, out)
	}

	manifestSrc := filepath.Join(pluginSrc, "plugin.json")
	manifestData, err := os.ReadFile(manifestSrc)
	if err != nil {
		t.Fatal(err)
	}
	pluginInstallDir := filepath.Join(outDir, "example-echo")
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

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestManifestRequiresSecretAccess(t *testing.T) {
	raw := `{"id":"com.example.secret","name":"x","version":"1","engine":{"type":"go-binary","entry":"a"},` +
		`"capabilities":{"vault":{"getSecret":["password"]}}}`
	var m domainplugin.Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if !m.RequiresSecretAccess() {
		t.Fatal("expected secret access")
	}
}
