package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

func TestProcessHostStartRejectsConcurrentStarting(t *testing.T) {
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})
	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID:      "com.test.concurrent",
			Name:    "Concurrent",
			Version: "1",
			Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "missing"},
		},
		RootDir: t.TempDir(),
	}
	key := processKey(plugin, "")

	host.mu.Lock()
	host.processes[key] = &managedProcess{
		key:    key,
		plugin: plugin,
		state:  domainplugin.ProcessStarting,
	}
	host.mu.Unlock()

	err := host.Start(context.Background(), plugin, "")
	if !errors.Is(err, domainplugin.ErrPluginAlreadyRunning) {
		t.Fatalf("expected ErrPluginAlreadyRunning, got %v", err)
	}
}

func TestProcessHostStartConcurrentOnlyOneReservation(t *testing.T) {
	pluginDir := buildSlowStartFixture(t)
	manifestData, err := os.ReadFile(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	plugin := domainplugin.InstalledPlugin{
		Manifest: manifest,
		RootDir:  pluginDir,
	}
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})

	const workers = 16
	var wg sync.WaitGroup
	var successes atomic.Int32
	var alreadyRunning atomic.Int32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			err := host.Start(context.Background(), plugin, "")
			if err == nil {
				successes.Add(1)
				return
			}
			if errors.Is(err, domainplugin.ErrPluginAlreadyRunning) {
				alreadyRunning.Add(1)
				return
			}
			t.Errorf("unexpected start error: %v", err)
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 successful start, got %d", got)
	}
	if got := alreadyRunning.Load(); got != workers-1 {
		t.Fatalf("expected %d ErrPluginAlreadyRunning, got %d", workers-1, got)
	}

	instances := host.RunningInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 running instance, got %d", len(instances))
	}
	if instances[0].State != domainplugin.ProcessRunning {
		t.Fatalf("expected running state, got %q", instances[0].State)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = host.Stop(ctx, manifest.ID, "")
}

func TestProcessHostStopDuringStarting(t *testing.T) {
	pluginDir := buildSlowStartFixture(t)
	manifestData, err := os.ReadFile(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	plugin := domainplugin.InstalledPlugin{
		Manifest: manifest,
		RootDir:  pluginDir,
	}
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})

	started := make(chan struct{})
	go func() {
		close(started)
		_ = host.Start(context.Background(), plugin, "")
	}()

	<-started
	deadline := time.Now().Add(2 * time.Second)
	for host.State(manifest.ID, "") != domainplugin.ProcessStarting {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for ProcessStarting")
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := host.Stop(ctx, manifest.ID, ""); err != nil {
		t.Fatalf("stop during starting: %v", err)
	}
	if st := host.State(manifest.ID, ""); st != domainplugin.ProcessDiscovered {
		t.Fatalf("expected discovered state after stop, got %q", st)
	}
}

// TestStartCancelledDuringHandshakeStillFailsAndReleases is the other half of "the child process is
// no longer owned by the caller's context": detaching the child must not make a cancelled start
// silently succeed. The caller's context still bounds the handshake, so cancelling it mid-initialize
// must still fail Start and still run the teardown — `running` stays false, so Start's deferred
// releaseStartReservation → finalizeProcess → closeResources(true) fires, which kills the child
// explicitly. That path predates this change and made the context's own kill redundant.
//
// What this test asserts is the failure and the released reservation, not the kill itself. It could
// not honestly assert the kill: every fixture exits on stdin EOF, and closeResources closes the
// connection before killing, so a process left unkilled here would leave no trace either. Proving
// the kill would need a fixture that ignores EOF; the reaper-level teardown itself is covered by
// TestProcessHostStopDuringStarting.
func TestStartCancelledDuringHandshakeStillFailsAndReleases(t *testing.T) {
	pluginDir := buildSlowStartFixture(t)
	manifestData, err := os.ReadFile(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	plugin := domainplugin.InstalledPlugin{Manifest: manifest, RootDir: pluginDir}
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})

	// The fixture sleeps 2s inside initialize, so this expires while the handshake is in flight —
	// after the process is up, which is the window the caller's context used to cover on its own.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := host.Start(ctx, plugin, ""); err == nil {
		t.Fatal("a Start whose context died mid-handshake must fail")
	}
	if st := host.State(manifest.ID, ""); st != domainplugin.ProcessDiscovered {
		t.Fatalf("a failed Start must release its reservation, got state %q", st)
	}
}

func buildSlowStartFixture(t *testing.T) string {
	t.Helper()
	root := repoRootForTest(t)
	pluginSrc := filepath.Join(root, "test", "fixtures", "plugin-slow-start")
	outDir := t.TempDir()

	manifestData, err := os.ReadFile(filepath.Join(pluginSrc, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	binName := manifest.Engine.Entry
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(outDir, binName)

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-trimpath", "-o", binPath, "./test/fixtures/plugin-slow-start")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build plugin-slow-start: %v\n%s", err, out)
	}

	pluginInstallDir := filepath.Join(outDir, "plugin-slow-start")
	if err := os.MkdirAll(pluginInstallDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginInstallDir, "plugin.json"), manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	binData, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginInstallDir, binName), binData, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := bundle.WriteChecksums(pluginInstallDir); err != nil {
		t.Fatal(err)
	}
	return pluginInstallDir
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
