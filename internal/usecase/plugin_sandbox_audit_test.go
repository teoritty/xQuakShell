package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// runningOnlyHost answers the one question the audit line is built from and nothing else. These
// tests are in package usecase because auditPluginSandbox is unexported, so they cannot reuse the
// stub the external test package already has.
type runningOnlyHost struct {
	instances []domainplugin.ProcessInstance
}

func (h *runningOnlyHost) RunningInstances() []domainplugin.ProcessInstance { return h.instances }

func (h *runningOnlyHost) Start(context.Context, domainplugin.InstalledPlugin, string, domainplugin.SandboxPolicy) error {
	return nil
}
func (h *runningOnlyHost) Stop(context.Context, string, string) error { return nil }
func (h *runningOnlyHost) StopAll(context.Context)                    {}
func (h *runningOnlyHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (h *runningOnlyHost) CallWithTimeout(context.Context, string, string, string, json.RawMessage, time.Duration) (json.RawMessage, error) {
	return nil, nil
}
func (h *runningOnlyHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (h *runningOnlyHost) State(string, string) domainplugin.ProcessState {
	return domainplugin.ProcessStopped
}
func (h *runningOnlyHost) BindSession(string, string) error { return nil }
func (h *runningOnlyHost) UnbindSession(string, string)     {}

// auditedStart is one line the manager wrote to the audit log.
type auditedStart struct {
	pluginID string
	reason   string
	detail   string
	denied   bool
}

func recordingManager(instances ...domainplugin.ProcessInstance) (*PluginManager, *[]auditedStart) {
	written := &[]auditedStart{}
	m := &PluginManager{host: &runningOnlyHost{instances: instances}}
	m.SetStartAudit(func(pluginID, reason, detail string, denied bool) {
		*written = append(*written, auditedStart{pluginID, reason, detail, denied})
	})
	return m, written
}

// TestTheAuditLineNamesTheModeTheProcessActuallyGot covers the one record that answers "was this
// plugin sandboxed", for each of the four answers.
//
// The unconfined ones matter most. An audit log that wrote a line only for the confined starts
// would answer the question with silence in exactly the case where it is worth asking, and silence
// reads as a missing entry rather than as a missing sandbox.
func TestTheAuditLineNamesTheModeTheProcessActuallyGot(t *testing.T) {
	modes := []domainplugin.SandboxMode{
		domainplugin.SandboxEnforced,
		domainplugin.SandboxEnforcedPartial,
		domainplugin.SandboxUnavailable,
		domainplugin.SandboxDisabled,
	}

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			m, written := recordingManager(domainplugin.ProcessInstance{
				PluginID: "com.example.p", SessionID: "sess-1",
				State: domainplugin.ProcessRunning, Sandbox: mode,
			})

			m.auditPluginSandbox("com.example.p", "sess-1")

			if len(*written) != 1 {
				t.Fatalf("wrote %d audit lines, want exactly 1; a start with no record of its "+
					"boundary is indistinguishable from one that was never audited", len(*written))
			}
			line := (*written)[0]
			if want := "sandbox=" + string(mode); line.detail != want {
				t.Errorf("detail = %q, want %q; the line has to name the mode this process got, "+
					"not the mode the platform could have given it", line.detail, want)
			}
			if line.pluginID != "com.example.p" || line.reason != "start" || line.denied {
				t.Errorf("line = %+v; want the start of com.example.p, not denied", line)
			}
		})
	}
}

// TestTheSandboxAuditLineCarriesNoPaths is a standing guard on what the record is allowed to say.
//
// The obvious way to make this line more useful is to append the directory that was granted, and
// that directory is under the user's home. An audit log is read, exported and pasted into bug
// reports (§2.2), so the line stays a mode and an identifier.
func TestTheSandboxAuditLineCarriesNoPaths(t *testing.T) {
	m, written := recordingManager(domainplugin.ProcessInstance{
		PluginID: "com.example.p", SessionID: "sess-1",
		State: domainplugin.ProcessRunning, Sandbox: domainplugin.SandboxEnforced,
	})

	m.auditPluginSandbox("com.example.p", "sess-1")

	for _, line := range *written {
		if strings.ContainsAny(line.detail, `/\`) {
			t.Errorf("audit detail %q contains a path separator", line.detail)
		}
	}
}

// TestNoAuditLineIsWrittenForAProcessThatIsNotRunning keeps the record from inventing one.
//
// sandboxModeForScope answers with an empty mode when it finds no matching instance — a stopped
// plugin, a session that never started, a scope mismatch. Writing "sandbox=" for that would put a
// claim in the log about a process that does not exist, and a log that records starts which never
// happened is worse than one that records nothing.
func TestNoAuditLineIsWrittenForAProcessThatIsNotRunning(t *testing.T) {
	cases := map[string]struct{ pluginID, scope string }{
		"nothing running at all": {"com.example.p", "sess-1"},
		"another plugin":         {"com.example.other", "sess-1"},
		"another session":        {"com.example.p", "sess-2"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var instances []domainplugin.ProcessInstance
			if name != "nothing running at all" {
				instances = append(instances, domainplugin.ProcessInstance{
					PluginID: "com.example.p", SessionID: "sess-1",
					State: domainplugin.ProcessRunning, Sandbox: domainplugin.SandboxEnforced,
				})
			}
			m, written := recordingManager(instances...)

			m.auditPluginSandbox(tc.pluginID, tc.scope)

			if len(*written) != 0 {
				t.Errorf("wrote %+v for a process that is not running", *written)
			}
		})
	}
}
