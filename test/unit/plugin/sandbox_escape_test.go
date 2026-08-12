package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
	"xquakshell/internal/infra/vault"
	"xquakshell/test/fixtures/escapeprobe"
)

const (
	escapeProbeID = "com.xquakshell.fixture-escape-probe"
	// otherPluginID stands in for anything else the user has installed. It never runs; what
	// matters is that its data directory exists and belongs to somebody else.
	otherPluginID = "com.xquakshell.fixture-other-plugin"
)

// want is what the confined run is required to show for one vector.
type want int

const (
	// mustBeDenied is the claim the sandbox badge makes. A vector marked this way failing is a
	// hole in the product, not in the test.
	mustBeDenied want = iota
	// knownGap is a vector the design admits it does not close — UDP is the standing example,
	// because Landlock's network rules cover TCP and nothing else. It is still sent, still run,
	// and still required to work unconfined, so the day it starts being denied the log says so.
	//
	// Leaving such a vector out of the table instead would be the same information loss the
	// enforced-partial mode exists to prevent: the gap would live only in prose.
	knownGap
)

// escapeVector is one attack, its target, and what the confined process must show for it.
type escapeVector struct {
	name   string
	target escapeprobe.Target
	want   want
	// why states the invariant in the failure message, so a red test names the rule it broke.
	why string
	// skip, when set, says this vector does not exist here and why. A skipped vector is reported
	// in the test log rather than dropped, because "not applicable" and "not checked" look
	// identical in a summary and only one of them is fine.
	skip string
}

func (v escapeVector) describe() string {
	switch {
	case v.target.Path != "":
		return v.target.Path
	case v.target.Addr != "":
		return v.target.Addr
	default:
		return fmt.Sprintf("pid %d", v.target.Pid)
	}
}

// escapeLayout is the world the probe is dropped into: the real installed-plugin directory shape,
// plus everything outside it that a plugin must not reach.
type escapeLayout struct {
	dataRoot    string
	installDir  string
	instanceDir string
	otherPlugin string
	foreignDir  string
	foreignFile string
	symlink     string
	dotdotPath  string
	vaultFile   string
	tcpAddr     string
	udpAddr     string
	parkedPid   int
	symlinkErr  error
}

// TestAnUnconfinedEscapeProbeReachesEverythingItTries is the control arm, and it is what makes the
// two tests below mean anything.
//
// Run the probe with no sandbox around it and every vector must get through. Without this, a probe
// with a typo in a path — or one whose fixture stopped being built, or one whose vector name drifted
// — reports "denied" forever and the confinement tests pass on a boundary that was never tested.
func TestAnUnconfinedEscapeProbeReachesEverythingItTries(t *testing.T) {
	_, _, layout := newProbeRig(t)
	vectors := escapeVectors(t, layout, sandbox.Support())

	request, err := json.Marshal(escapeprobe.Request{Vectors: requestedVectors(vectors)})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(probeBinaryIn(layout.installDir), string(request)).Output()
	if err != nil {
		t.Fatalf("run the probe directly: %v", err)
	}
	var report escapeprobe.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse the probe report %q: %v", out, err)
	}

	for _, vector := range vectors {
		if vector.skip != "" {
			t.Logf("%s is not attempted here: %s", vector.name, vector.skip)
			continue
		}
		outcome := report.Vectors[vector.name]
		requireAttempted(t, vector, outcome)
		if !outcome.Succeeded {
			t.Errorf("unconfined %s failed against %s (%s); the confined tests below would pass "+
				"whether or not a sandbox existed", vector.name, vector.describe(), outcome.Err)
		}
	}
}

// TestAConfinedPluginCannotReachOutsideItsOwnDirectories is the claim the sandbox badge makes,
// tested the only way it can honestly be tested: by trying.
func TestAConfinedPluginCannotReachOutsideItsOwnDirectories(t *testing.T) {
	support := requireSandbox(t)
	host, plugin, layout := newProbeRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := host.Start(ctx, plugin, "sess-a", domainplugin.SandboxPolicy{}); err != nil {
		t.Fatalf("start the probe plugin: %v", err)
	}
	defer host.StopAll(context.Background())

	vectors := escapeVectors(t, layout, support)
	report := runProbe(ctx, t, host, "sess-a", requestedVectors(vectors))

	for _, vector := range vectors {
		if vector.skip != "" {
			t.Logf("%s is not checked here: %s", vector.name, vector.skip)
			continue
		}
		outcome := report.Vectors[vector.name]
		requireAttempted(t, vector, outcome)
		// The refusal itself, kept in the verbose log. Which door closed is not obvious from a
		// green run — a denial can come from the ruleset, from the job object, or from a path that
		// was never reachable — and reading it is how a vector gets caught testing the wrong thing.
		t.Logf("%s against %s: succeeded=%v (%s)", vector.name, vector.describe(), outcome.Succeeded, outcome.Err)

		if vector.want == knownGap {
			// Recorded, never asserted in either direction: requiring the gap to stay open would
			// fail the day a newer kernel closes it, and requiring it to be closed would fail
			// today on a platform this code is honest about.
			t.Logf("  ...and %s is a gap this design admits: %s", vector.name, support.Reason)
			continue
		}
		if outcome.Succeeded {
			t.Errorf("the plugin got through %s against %s; %s", vector.name, vector.describe(), vector.why)
		}
	}
}

// TestOneSessionOfAPluginCannotReadAnothersInstanceDirectory is the direct test of the hazard that
// nearly went unnoticed: the plugin's data directory sits inside its install tree, so granting the
// install directory as one path would hand every session read access to every other session's
// files — through the mechanism added to prevent exactly that.
func TestOneSessionOfAPluginCannotReadAnothersInstanceDirectory(t *testing.T) {
	requireSandbox(t)
	host, plugin, layout := newProbeRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for _, session := range []string{"sess-a", "sess-b"} {
		if err := host.Start(ctx, plugin, session, domainplugin.SandboxPolicy{}); err != nil {
			t.Fatalf("start the probe plugin for %s: %v", session, err)
		}
	}
	defer host.StopAll(context.Background())

	// A file only session b's process is supposed to be able to see, written by the test rather
	// than by that process so its absence cannot be mistaken for the assertion passing.
	otherDir := infraplugin.PluginInstanceDataDir(layout.dataRoot, escapeProbeID, "sess-b", domainplugin.IsolationPerSession)
	secret := filepath.Join(otherDir, "session-secret")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed the other session's directory: %v", err)
	}

	report := runProbe(ctx, t, host, "sess-a", map[string]escapeprobe.Target{
		escapeprobe.VectorReadForeign:  {Path: secret},
		escapeprobe.VectorWriteForeign: {Path: otherDir},
	})
	if report.Vectors[escapeprobe.VectorReadForeign].Succeeded {
		t.Errorf("session a read %s; granting the plugin's base directory instead of its instance "+
			"directory is what this catches", secret)
	}
	if report.Vectors[escapeprobe.VectorWriteForeign].Succeeded {
		t.Errorf("session a wrote into session b's directory %s", otherDir)
	}
}

// escapeVectors is the whole attack surface this probe covers, in one place, with the platform
// questions answered once rather than at each assertion.
func escapeVectors(t *testing.T, layout escapeLayout, support domainplugin.SandboxSupport) []escapeVector {
	t.Helper()

	// A platform that confines the network confines all of it; Landlock confines TCP at best and
	// reports Network false for exactly that reason, so both socket vectors take their expectation
	// from the same published fact rather than from a guess about the kernel.
	socket := knownGap
	if support.Network {
		socket = mustBeDenied
	}

	vectors := []escapeVector{{
		name:   escapeprobe.VectorReadForeign,
		target: escapeprobe.Target{Path: layout.foreignFile},
		want:   mustBeDenied,
		why:    "a confined plugin must not reach a file outside its own directories",
	}, {
		name:   escapeprobe.VectorWriteForeign,
		target: escapeprobe.Target{Path: layout.foreignDir},
		want:   mustBeDenied,
		why:    "a confined plugin must not create files outside its own directories",
	}, {
		name:   escapeprobe.VectorReadVault,
		target: escapeprobe.Target{Path: layout.vaultFile},
		want:   mustBeDenied,
		why:    "the vault is the thing this sandbox exists to keep away from plugins",
	}, {
		name:   escapeprobe.VectorReadOtherPlugin,
		target: escapeprobe.Target{Path: filepath.Join(layout.otherPlugin, "their-secret")},
		want:   mustBeDenied,
		why:    "one plugin's data is not another plugin's to read",
	}, {
		name:   escapeprobe.VectorListPluginsRoot,
		target: escapeprobe.Target{Path: infraplugin.PluginsRoot(layout.dataRoot)},
		want:   mustBeDenied,
		why:    "listing the install root tells a plugin what else the user runs",
	}, {
		name:   escapeprobe.VectorDotDotTraversal,
		target: escapeprobe.Target{Path: layout.dotdotPath},
		want:   mustBeDenied,
		why:    "the boundary must survive a path that climbs out of it rather than naming its way out",
	}, {
		name:   escapeprobe.VectorSymlinkEscape,
		target: escapeprobe.Target{Path: layout.symlink},
		want:   mustBeDenied,
		why:    "a link inside a granted directory must be judged by where it lands, not where it lives",
		skip:   symlinkSkip(layout),
	}, {
		name:   escapeprobe.VectorWriteThenExec,
		target: escapeprobe.Target{Path: layout.instanceDir},
		want:   mustBeDenied,
		// Two independent guards have to hold for this to be denied, and each one alone is enough:
		// the writable grant carries no execute right, and on Windows the job object caps the
		// plugin at one process. Removing either on its own leaves this green, which is what
		// defence in depth is supposed to look like — and is why the failure names the property
		// rather than the mechanism that happened to catch it.
		why: "a plugin that can write a binary and then run it is a loader for arbitrary code",
	}, {
		name:   escapeprobe.VectorWriteInstallTree,
		target: escapeprobe.Target{Path: layout.installDir},
		want:   mustBeDenied,
		why:    "the install tree is granted for reading and running, never for writing",
	}, {
		name:   escapeprobe.VectorDialTCP,
		target: escapeprobe.Target{Addr: layout.tcpAddr},
		want:   socket,
		why:    "this platform reports network confinement",
	}, {
		name:   escapeprobe.VectorDialUDP,
		target: escapeprobe.Target{Addr: layout.udpAddr},
		want:   socket,
		why:    "this platform reports network confinement",
	}}

	return append(vectors, linuxOnlyVectors(t, layout)...)
}

// linuxOnlyVectors are the escapes that exist only where /proc and ptrace do. They are declared on
// every platform and skipped with a reason elsewhere, so that "Windows has no /proc" stays visible
// in the output instead of being invisible in a build tag.
func linuxOnlyVectors(t *testing.T, layout escapeLayout) []escapeVector {
	t.Helper()
	notLinux := ""
	if runtime.GOOS != "linux" {
		notLinux = "there is no /proc or ptrace on " + runtime.GOOS
	}

	return []escapeVector{{
		name:   escapeprobe.VectorProcEnviron,
		target: escapeprobe.Target{Pid: os.Getpid()},
		want:   mustBeDenied,
		why:    "the host's environment is readable through /proc, and no rule grants /proc",
		skip:   notLinux,
	}, {
		name:   escapeprobe.VectorProcSelfRoot,
		target: escapeprobe.Target{Path: layout.foreignFile},
		want:   mustBeDenied,
		why:    "/proc/self/root is a second name for /, and a path-based sandbox must resolve it",
		skip:   notLinux,
	}, {
		name:   escapeprobe.VectorPtraceHost,
		target: escapeprobe.Target{Pid: layout.parkedPid},
		want:   mustBeDenied,
		why:    "attaching to a process outside the sandbox hands over its memory, rules or no rules",
		skip:   ptraceSkip(notLinux),
	}}
}

// ptraceSkip refuses to let Yama answer this test's question for it.
//
// With ptrace_scope at 1 — the default nearly everywhere — attaching to anything that is not a
// descendant is refused by Yama before the sandbox is consulted, so a green assertion would prove
// nothing about this code. The skip says so out loud rather than banking a free pass.
func ptraceSkip(notLinux string) string {
	if notLinux != "" {
		return notLinux
	}
	raw, err := os.ReadFile("/proc/sys/kernel/yama/ptrace_scope")
	if err != nil {
		// No Yama: the kernel's own restriction is absent, so the attach is a fair question.
		return ""
	}
	if scope := strings.TrimSpace(string(raw)); scope != "0" {
		return "Yama ptrace_scope is " + scope + ", so the kernel refuses this attach before the " +
			"sandbox is asked; the sandbox's own ptrace boundary stays unverified on this machine"
	}
	return ""
}

func symlinkSkip(layout escapeLayout) string {
	if layout.symlinkErr != nil {
		return "this machine would not let the test create a symlink (" + layout.symlinkErr.Error() +
			"), so the link-following boundary stays unverified here"
	}
	return ""
}

// requestedVectors is what actually goes on the wire: every vector that applies here.
func requestedVectors(vectors []escapeVector) map[string]escapeprobe.Target {
	requested := make(map[string]escapeprobe.Target, len(vectors))
	for _, vector := range vectors {
		if vector.skip == "" {
			requested[vector.name] = vector.target
		}
	}
	return requested
}

// requireAttempted is the guard against a vacuous pass. An outcome that was never attempted is
// indistinguishable from a denial in every field the assertions read, so a vector the fixture does
// not know — a renamed constant, a stale build — would otherwise report the sandbox working.
func requireAttempted(t *testing.T, vector escapeVector, outcome escapeprobe.Outcome) {
	t.Helper()
	if !outcome.Attempted {
		t.Fatalf("the probe did not attempt %s (%s); an unattempted vector reads exactly like a "+
			"denied one, so nothing below can be trusted", vector.name, outcome.Err)
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

func newProbeRig(t *testing.T) (*infraplugin.ProcessHost, domainplugin.InstalledPlugin, escapeLayout) {
	t.Helper()
	// The data root and the world outside it share one parent so that the ".." vector can be
	// built as a real relative climb instead of a path with a guessed number of levels in it.
	base := t.TempDir()
	dataRoot := filepath.Join(base, "app-data")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	pluginDir := installProbeWhereARealOneLives(t, dataRoot)
	host := infraplugin.NewProcessHost(infraplugin.HostConfig{DataRoot: dataRoot})

	var manifest domainplugin.Manifest
	if err := json.Unmarshal(readFile(t, filepath.Join(pluginDir, "plugin.json")), &manifest); err != nil {
		t.Fatalf("read the probe manifest: %v", err)
	}
	plugin := domainplugin.InstalledPlugin{Manifest: manifest, RootDir: pluginDir}
	return host, plugin, newEscapeLayout(t, base, dataRoot, pluginDir)
}

// newEscapeLayout builds every target once, for both arms of the test. The confined run and the
// control run must attack the same shapes, or the control run stops being evidence about the
// confined one.
func newEscapeLayout(t *testing.T, base, dataRoot, installDir string) escapeLayout {
	t.Helper()

	instanceDir, err := infraplugin.EnsurePluginInstanceDataDir(dataRoot, escapeProbeID, "sess-a",
		domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatalf("create the probe's own data directory: %v", err)
	}
	otherPlugin, err := infraplugin.EnsurePluginInstanceDataDir(dataRoot, otherPluginID, "sess-a",
		domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatalf("create the other plugin's data directory: %v", err)
	}
	writeSeed(t, filepath.Join(otherPlugin, "their-secret"))

	foreignDir := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignFile := filepath.Join(foreignDir, "not-yours")
	writeSeed(t, foreignFile)

	// The vault path comes from the code that computes it in production, over the same data root
	// the host was given. A literal "vault.age" here would keep passing after the real one moved.
	vaultFile := vault.FilePath(dataRoot)
	writeSeed(t, vaultFile)

	symlink := filepath.Join(instanceDir, "escape-link")
	symlinkErr := os.Symlink(foreignFile, symlink)

	return escapeLayout{
		dataRoot:    dataRoot,
		installDir:  installDir,
		instanceDir: instanceDir,
		otherPlugin: otherPlugin,
		foreignDir:  foreignDir,
		foreignFile: foreignFile,
		symlink:     symlink,
		symlinkErr:  symlinkErr,
		dotdotPath:  climbOutOf(t, instanceDir, foreignFile),
		vaultFile:   vaultFile,
		tcpAddr:     startTCPListener(t),
		udpAddr:     startUDPEcho(t),
		parkedPid:   startParkedProcess(t, installDir),
	}
}

// climbOutOf spells a path from inside the sandbox to a file outside it using "..", leaving the
// dots unresolved so the kernel is the one that has to resolve them.
func climbOutOf(t *testing.T, from, target string) string {
	t.Helper()
	rel, err := filepath.Rel(from, target)
	if err != nil {
		t.Fatalf("build a traversal from %s to %s: %v", from, target, err)
	}
	if !strings.HasPrefix(rel, "..") {
		t.Fatalf("%s does not climb out of %s; this vector would test nothing", rel, from)
	}
	return from + string(filepath.Separator) + rel
}

// The vault stand-in and the other plugin's file hold no secret on purpose. The test needs a file
// at the path production would use, not a file worth stealing: a fixture that pointed at a real
// vault would read a developer's keys on every run.
func writeSeed(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("not a real secret"), 0o600); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
}

func startTCPListener(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener.Addr().String()
}

// startUDPEcho answers datagrams, because a UDP socket that is created and never used says nothing
// about whether packets can leave. The probe requires the echo back.
func startUDPEcho(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 64)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return // the cleanup above closed it; there is nothing else to report to
			}
			_, _ = conn.WriteTo(buf[:n], from)
		}
	}()
	return conn.LocalAddr().String()
}

// startParkedProcess offers the ptrace vector something expendable to attach to. The test process
// itself is the wrong victim: an attach that succeeded and then failed to detach would leave the
// whole suite stopped, and a flake that hangs CI is worse than the coverage is worth.
func startParkedProcess(t *testing.T, installDir string) int {
	t.Helper()
	parked := exec.Command(probeBinaryIn(installDir), "--park")
	if err := parked.Start(); err != nil {
		t.Fatalf("park a process for the ptrace vector: %v", err)
	}
	t.Cleanup(func() {
		_ = parked.Process.Kill()
		_ = parked.Wait()
	})
	return parked.Process.Pid
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

// probeBinaryIn names the installed binary, so the control arm runs the very file the confined arm
// runs rather than a second build of the same source.
func probeBinaryIn(installDir string) string {
	entry := "plugin-escape-probe"
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	return filepath.Join(installDir, entry)
}

func runProbe(ctx context.Context, t *testing.T, host *infraplugin.ProcessHost, session string,
	targets map[string]escapeprobe.Target) escapeprobe.Report {
	t.Helper()
	params, err := json.Marshal(escapeprobe.Request{Vectors: targets})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := host.Call(ctx, escapeProbeID, session, "probe.run", params)
	if err != nil {
		t.Fatalf("ask the probe to run: %v", err)
	}
	var report escapeprobe.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse the probe report %q: %v", raw, err)
	}
	return report
}
