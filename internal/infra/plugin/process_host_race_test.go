package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

// TestStopDuringStartingLeavesNoLiveProcess is the same scenario as the test above, asked of the OS
// instead of the host — and the two answers used to differ.
//
// Stop takes the reservation, sets ProcessStopping and then reads mp.cmd/mp.reaper, which are nil
// until Start has spawned AND published. A Stop landing in that window therefore kills nothing,
// while finalizeProcess drops mp from the registry and burns cleanupOnce, so the teardown can never
// run again. For as long as the child was owned by the caller's context it died anyway — as a side
// effect, not by design. Detaching it (the point of this change) removed that accident and turned
// the same window into an orphan: a live plugin process nobody is tracking, with a Windows job
// handle that will never be closed, and a waitProcess goroutine parked forever on its reaper.
//
// The assertion is by pid, deliberately. The test above asks the host and is satisfied by
// ProcessDiscovered, which the orphan produces too — that blindness is why the regression could be
// introduced under a green suite.
func TestStopDuringStartingLeavesNoLiveProcess(t *testing.T) {
	pluginDir := buildSlowStartFixture(t)
	manifest := readFixtureManifest(t, pluginDir)
	plugin := domainplugin.InstalledPlugin{Manifest: manifest, RootDir: pluginDir}
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})

	startDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		startDone <- host.Start(context.Background(), plugin, "")
	}()

	// Same timing as TestProcessHostStopDuringStarting: ProcessStarting is set on Start's first
	// lines, so Stop lands while the manifest negotiation and the bundle checksum walk are still
	// running — before the process exists, let alone before it is published.
	<-started
	deadline := time.Now().Add(5 * time.Second)
	for host.State(manifest.ID, "") != domainplugin.ProcessStarting {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for ProcessStarting")
		}
		time.Sleep(5 * time.Millisecond)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := host.Stop(stopCtx, manifest.ID, ""); err != nil {
		t.Fatalf("stop during starting: %v", err)
	}

	var startErr error
	select {
	case startErr = <-startDone:
	case <-time.After(30 * time.Second):
		t.Fatal("Start never returned after a concurrent Stop")
	}

	// Liveness is checked first, so that a regression is reported as the orphan it is rather than as
	// a bookkeeping detail.
	const what = "a Stop that arrived while the plugin was still starting"
	pid, announced := fixturePID(t, pluginDir)
	if announced {
		assertProcessGone(t, pid, what)
	}

	// A Stop that took the reservation must not leave Start reporting success.
	if startErr == nil {
		t.Fatal("Start reported success for a plugin whose reservation Stop had already taken")
	}
	// The kill fires within milliseconds of exec, often before the child runs its first statement, so
	// no pid at all is a normal outcome — but only errStartAbortedByStop proves a child was spawned
	// and discarded rather than never spawned, which is what keeps this test from passing vacuously.
	if !announced && !errors.Is(startErr, errStartAbortedByStop) {
		t.Fatalf("no pid was announced and Start failed with %v, so %s cannot be judged", startErr, what)
	}
}

// TestStartCancelledDuringHandshakeLeavesNoLiveProcess is the other half of "the child is no longer
// owned by the caller's context": that context still bounds the HANDSHAKE, so cancelling it
// mid-initialize must still fail Start and still take the process down — now through
// releaseStartReservation → finalizeProcess → closeResources(true) alone, with no context kill
// behind it.
//
// It pins the OUTCOME, not the mechanism, and the distinction is worth stating because it was
// measured: with the explicit kill and the job close both disabled, this test still passes, because
// closeResources shuts the connection first and every fixture exits on stdin EOF. What it does
// guarantee is the thing that matters to a user — an abandoned start leaves no plugin process
// behind — and it would catch a teardown that stopped closing the connection as well.
func TestStartCancelledDuringHandshakeLeavesNoLiveProcess(t *testing.T) {
	pluginDir := buildSlowStartFixture(t)
	manifest := readFixtureManifest(t, pluginDir)
	plugin := domainplugin.InstalledPlugin{Manifest: manifest, RootDir: pluginDir}
	host := NewProcessHost(HostConfig{DataRoot: t.TempDir()})

	// The fixture sleeps 2s inside initialize, so this expires while the handshake is in flight —
	// after the process is up and published, which is the window the caller's context used to cover.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := host.Start(ctx, plugin, ""); err == nil {
		t.Fatal("a Start whose context died mid-handshake must fail")
	}
	if st := host.State(manifest.ID, ""); st != domainplugin.ProcessDiscovered {
		t.Fatalf("a failed Start must release its reservation, got state %q", st)
	}
	// Here the child certainly ran — the handshake reached it — so a missing pid would be a broken
	// fixture, not a fast kill.
	pid, ok := fixturePID(t, pluginDir)
	if !ok {
		t.Fatal("the fixture never announced a pid, yet its handshake was under way")
	}
	assertProcessGone(t, pid, "a Start whose context was cancelled mid-handshake")
}

func readFixtureManifest(t *testing.T, pluginDir string) domainplugin.Manifest {
	t.Helper()
	manifestData, err := os.ReadFile(filepath.Join(pluginDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// fixturePID reads the pid the slow-start fixture announced. Taking it from the child rather than
// from the host is the whole point: a host that lost a process mid-start has no pid to give, which
// is precisely the case being tested.
func fixturePID(t *testing.T, pluginDir string) (int, bool) {
	t.Helper()
	pidPath := filepath.Join(filepath.Dir(pluginDir), "slow-start.pid")

	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(pidPath); err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid, true
			}
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// assertProcessGone waits for an OS process to disappear, asking the kernel and not the host.
func assertProcessGone(t *testing.T, pid int, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for processAliveForTest(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("%s left plugin process %d running with nothing tracking it", what, pid)
		}
		time.Sleep(50 * time.Millisecond)
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
