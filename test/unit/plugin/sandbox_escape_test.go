package plugin_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

const escapeProbeID = "com.xquakshell.fixture-escape-probe"

// probeReport mirrors what test/fixtures/plugin-escape-probe reports back. Only the fields the
// assertions read are declared; the fixture's error strings are for the failure message.
type probeReport struct {
	Read  probeOutcome `json:"read"`
	Write probeOutcome `json:"write"`
	Dial  probeOutcome `json:"dial"`
}

type probeOutcome struct {
	Attempted bool   `json:"attempted"`
	Denied    bool   `json:"denied"`
	Err       string `json:"err,omitempty"`
}

// escapeTargets is what the probe is told to attack. Nothing here is a real path on the developer's
// machine: a fixture that knew where the SSH keys live would read them on every test run.
type escapeTargets struct {
	ReadPath string `json:"readPath"`
	WriteDir string `json:"writeDir"`
	DialAddr string `json:"dialAddr"`
}

// TestAnUnconfinedEscapeProbeReachesEverythingItTries is the control arm, and it is what makes the
// two tests below mean anything.
//
// Run the probe with no sandbox around it and all three attempts must succeed. Without this, a
// probe with a typo in a path — or one whose fixture stopped being built — reports "denied" forever
// and the confinement tests pass on a boundary that was never tested.
func TestAnUnconfinedEscapeProbeReachesEverythingItTries(t *testing.T) {
	binary := buildProbeBinary(t)
	targets, listener := newEscapeTargets(t)
	defer func() { _ = listener.Close() }()

	request, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(binary, string(request)).Output()
	if err != nil {
		t.Fatalf("run the probe directly: %v", err)
	}
	var report probeReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse the probe report %q: %v", out, err)
	}

	for name, outcome := range map[string]probeOutcome{"read": report.Read, "write": report.Write, "dial": report.Dial} {
		if !outcome.Attempted {
			t.Errorf("%s was not attempted; the probe is not probing what the confined tests assume", name)
		}
		if outcome.Denied || outcome.Err != "" {
			t.Errorf("unconfined %s failed (%s); the confined tests below would pass whether or not "+
				"a sandbox existed", name, outcome.Err)
		}
	}
}

// TestAConfinedPluginCannotReachOutsideItsOwnDirectories is the claim the sandbox badge makes,
// tested the only way it can honestly be tested: by trying.
func TestAConfinedPluginCannotReachOutsideItsOwnDirectories(t *testing.T) {
	support := requireSandbox(t)
	host, plugin, _ := newProbeRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := host.Start(ctx, plugin, "sess-a"); err != nil {
		t.Fatalf("start the probe plugin: %v", err)
	}
	defer host.StopAll(context.Background())

	targets, listener := newEscapeTargets(t)
	defer func() { _ = listener.Close() }()
	report := runProbe(ctx, t, host, "sess-a", targets)

	if !report.Read.Denied {
		t.Errorf("the plugin read %s (err %q); a confined plugin must not reach a file outside "+
			"its own directories", targets.ReadPath, report.Read.Err)
	}
	if !report.Write.Denied {
		t.Errorf("the plugin wrote into %s (err %q)", targets.WriteDir, report.Write.Err)
	}
	// The network is reported separately because the platform delivers it separately: a kernel
	// below Landlock ABI 4 has no network rules at all, and enforced-partial is exactly that
	// admission. Asserting denial unconditionally would fail on a machine the code is honest about.
	if support.Network && !report.Dial.Denied {
		t.Errorf("the plugin connected to %s; this platform reports network confinement", targets.DialAddr)
	}
}

// TestOneSessionOfAPluginCannotReadAnothersInstanceDirectory is the direct test of the hazard that
// nearly went unnoticed: the plugin's data directory sits inside its install tree, so granting the
// install directory as one path would hand every session read access to every other session's
// files — through the mechanism added to prevent exactly that.
func TestOneSessionOfAPluginCannotReadAnothersInstanceDirectory(t *testing.T) {
	requireSandbox(t)
	host, plugin, dataRoot := newProbeRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, session := range []string{"sess-a", "sess-b"} {
		if err := host.Start(ctx, plugin, session); err != nil {
			t.Fatalf("start the probe plugin for %s: %v", session, err)
		}
	}
	defer host.StopAll(context.Background())

	// A file only session b's process is supposed to be able to see, written by the test rather
	// than by that process so its absence cannot be mistaken for the assertion passing.
	otherDir := infraplugin.PluginInstanceDataDir(dataRoot, escapeProbeID, "sess-b", domainplugin.IsolationPerSession)
	secret := filepath.Join(otherDir, "session-secret")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed the other session's directory: %v", err)
	}

	report := runProbe(ctx, t, host, "sess-a", escapeTargets{ReadPath: secret, WriteDir: otherDir})
	if !report.Read.Denied {
		t.Errorf("session a read %s (err %q); granting the plugin's base directory instead of its "+
			"instance directory is what this catches", secret, report.Read.Err)
	}
	if !report.Write.Denied {
		t.Errorf("session a wrote into session b's directory %s (err %q)", otherDir, report.Write.Err)
	}
}

func requireSandbox(t *testing.T) domainplugin.SandboxSupport {
	t.Helper()
	support := sandbox.Support()
	if !support.Available {
		t.Skipf("this platform confines nothing: %s", support.Reason)
	}
	return support
}

func newProbeRig(t *testing.T) (*infraplugin.ProcessHost, domainplugin.InstalledPlugin, string) {
	t.Helper()
	dataRoot := t.TempDir()
	pluginDir := installProbeWhereARealOneLives(t, dataRoot)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: dataRoot})

	var manifest domainplugin.Manifest
	if err := json.Unmarshal(readFile(t, filepath.Join(pluginDir, "plugin.json")), &manifest); err != nil {
		t.Fatalf("read the probe manifest: %v", err)
	}
	return host, domainplugin.InstalledPlugin{Manifest: manifest, RootDir: pluginDir}, dataRoot
}

// installProbeWhereARealOneLives puts the fixture at <dataRoot>/plugins/<id>, which is where an
// installed plugin actually sits.
//
// The layout is the test, not scaffolding around it. A plugin's data directory is a CHILD of its
// own install directory there, so a sandbox that granted the install tree as one path would carry
// every session's data along with it. The obvious rig — build the fixture into one temp directory
// and point the host's data root at another — has no such nesting, so the cross-session test passes
// with the bug fully present. It did, until the mutation that grants the whole install directory
// failed to turn it red.
func installProbeWhereARealOneLives(t *testing.T, dataRoot string) string {
	t.Helper()
	built := buildFixturePlugin(t, "plugin-escape-probe")
	installed := filepath.Join(infraplugin.PluginsRoot(dataRoot), escapeProbeID)
	if err := os.MkdirAll(installed, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(built)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(installed, entry.Name())
		if err := os.WriteFile(dest, readFile(t, filepath.Join(built, entry.Name())), info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	return installed
}

// buildProbeBinary produces the same binary the plugin rig installs, at a path the test can run
// itself. Reusing buildFixturePlugin keeps the two arms of this file honest: the control run and
// the confined run are the same code, built the same way.
func buildProbeBinary(t *testing.T) string {
	t.Helper()
	entry := "plugin-escape-probe"
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	return filepath.Join(buildFixturePlugin(t, "plugin-escape-probe"), entry)
}

// newEscapeTargets picks paths and an address outside anything a plugin is granted, and returns the
// listener so the dial has something real to reach when nothing stops it.
func newEscapeTargets(t *testing.T) (escapeTargets, net.Listener) {
	t.Helper()
	outside := t.TempDir()
	readPath := filepath.Join(outside, "not-yours")
	if err := os.WriteFile(readPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return escapeTargets{ReadPath: readPath, WriteDir: outside, DialAddr: listener.Addr().String()}, listener
}

func runProbe(ctx context.Context, t *testing.T, host *infraplugin.ProcessHost, session string, targets escapeTargets) probeReport {
	t.Helper()
	params, err := json.Marshal(targets)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := host.Call(ctx, escapeProbeID, session, "probe.run", params)
	if err != nil {
		t.Fatalf("ask the probe to run: %v", err)
	}
	var report probeReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse the probe report %q: %v", raw, err)
	}
	return report
}
