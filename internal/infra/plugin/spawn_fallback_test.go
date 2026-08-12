package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// TestTheFallbackPolicyDecidesTheSameWayForEveryCombination is the policy table for what happens
// when a confinement does not apply.
//
// Three inputs decide it: can the platform confine, did the attempt fail, and did the user allow a
// fallback. The row that matters most is "platform can confine / attempt failed / not allowed",
// which must REFUSE. A silent downgrade there is how a sandbox stops working for a fraction of
// users with nobody noticing, and it is indistinguishable from success in every log and every
// badge.
func TestTheFallbackPolicyDecidesTheSameWayForEveryCombination(t *testing.T) {
	cases := []struct {
		name        string
		allow       bool
		attemptErr  error
		wantRefusal bool
		wantMode    domainplugin.SandboxMode
		why         string
	}{
		{
			name:        "confinement failed and the user did not allow a fallback",
			attemptErr:  errors.New("the ruleset would not apply"),
			wantRefusal: true,
			why:         "refusing is the only answer that cannot be mistaken for a working sandbox",
		},
		{
			name:       "confinement failed and the user allowed a fallback",
			allow:      true,
			attemptErr: errors.New("the ruleset would not apply"),
			wantMode:   domainplugin.SandboxDisabled,
			why:        "the process really is unconfined and the mode has to say so",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := fallbackRequest(t, tc.allow)
			started, err := fallBackOrRefuse(req, tc.attemptErr)

			if tc.wantRefusal {
				if !errors.Is(err, domainplugin.ErrSandboxUnavailable) {
					t.Fatalf("error = %v, want %v; %s", err, domainplugin.ErrSandboxUnavailable, tc.why)
				}
				if started.child != nil {
					t.Error("a refusal started a process anyway")
				}
				return
			}
			if err != nil {
				t.Fatalf("the allowed fallback failed to start the plugin: %v", err)
			}
			t.Cleanup(func() { _ = started.child.Kill() })
			if started.mode != tc.wantMode {
				t.Errorf("mode = %q, want %q; %s", started.mode, tc.wantMode, tc.why)
			}
		})
	}
}

// TestAPlatformThatCannotConfineNeverReachesThePolicy is the other half, and the reason the two
// cases do not share a code path.
//
// "This platform has no sandbox" is not a failure and needs no opt-in: macOS and a kernel without
// Landlock start plugins exactly as they always have. If that case ever arrived at the policy, a
// user on such a platform would need the fallback setting turned on to run any plugin at all — and
// turning it on would then also silence the case where a working sandbox breaks.
func TestAPlatformThatCannotConfineNeverReachesThePolicy(t *testing.T) {
	if sandbox.Support().Available {
		t.Skip("this platform can confine a plugin, so it is the other half of the table")
	}
	req := fallbackRequest(t, false)

	started, err := startPluginChild(req)
	if err != nil {
		t.Fatalf("a platform that cannot confine refused to start a plugin: %v", err)
	}
	t.Cleanup(func() { _ = started.child.Kill() })

	if started.mode != domainplugin.SandboxUnavailable {
		t.Errorf("mode = %q, want %q", started.mode, domainplugin.SandboxUnavailable)
	}
}

// fallbackRequest builds a request around a real binary, because the allowed-fallback path really
// does start a process.
//
// It is a system shell rather than a fixture plugin: nothing here speaks the protocol or is
// expected to, the assertion is about which decision was taken, and building a fixture would make a
// pure-logic test depend on the go tool. The child is killed in cleanup.
func fallbackRequest(t *testing.T, allow bool) childRequest {
	t.Helper()
	exe := harmlessExecutable(t)
	dataRoot := t.TempDir()
	instanceDir := filepath.Join(dataRoot, "instance")
	if err := os.MkdirAll(instanceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return childRequest{
		dataRoot:        dataRoot,
		plugin:          domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{ID: "com.test.policy"}, RootDir: filepath.Dir(exe)},
		entryPath:       exe,
		instanceDataDir: instanceDir,
		env:             []string{},
		stderr:          nopWriteCloser{},
		policy:          domainplugin.SandboxPolicy{AllowUnsandboxedFallback: allow},
	}
}

// harmlessExecutable is a binary that exists on every machine the suite runs on and that starting
// costs nothing.
func harmlessExecutable(t *testing.T) string {
	t.Helper()
	candidate := "/bin/sh"
	if runtime.GOOS == "windows" {
		candidate = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Skipf("no harmless executable to stand in for a plugin: %v", err)
	}
	return candidate
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }
