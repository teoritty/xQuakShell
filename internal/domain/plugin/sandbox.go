package plugin

// SandboxMode reports what OS-level boundary a plugin process is actually running behind.
//
// It describes an outcome, never an intention. A plugin process that reports SandboxEnforced is one
// the operating system is holding inside its own directories; anything else means the process has
// the user's full reach, and the capability gate — which is an IPC boundary, not a syscall one — is
// all that stands between the plugin and the rest of the machine.
type SandboxMode string

const (
	// SandboxEnforced means the OS is confining this process.
	SandboxEnforced SandboxMode = "enforced"

	// SandboxUnavailable means this platform, kernel or build cannot confine it. This is not a
	// failure and does not stop a plugin from starting: it is the honest state of macOS, of a Linux
	// kernel without Landlock, and of every platform in builds where the isolation is not yet
	// implemented.
	SandboxUnavailable SandboxMode = "unavailable"

	// SandboxDisabled means the platform can confine the process and the user chose not to.
	SandboxDisabled SandboxMode = "disabled"
)

// SandboxSupport is what a platform can do, asked once per build and independent of any plugin.
type SandboxSupport struct {
	// Available reports whether this build on this machine can confine a plugin process.
	Available bool
	// Reason says why not, for the log and the UI. It is empty when Available is true, and it is
	// the whole value of this type: "unavailable" without a reason sends a user looking through
	// settings for a switch that does not exist.
	Reason string
}

// Mode is the mode a process gets on a platform with this support and nothing else deciding.
func (s SandboxSupport) Mode() SandboxMode {
	if s.Available {
		return SandboxEnforced
	}
	return SandboxUnavailable
}

// sandboxStrength orders the modes so that the weakest one can be picked out of a set. Enforced is
// the only one that confines anything; the other two differ in why they do not.
func sandboxStrength(m SandboxMode) int {
	switch m {
	case SandboxEnforced:
		return 2
	case SandboxDisabled:
		return 1
	default:
		return 0
	}
}

// WeakestSandboxMode reduces the modes of several processes to the one worth reporting for the
// plugin they belong to.
//
// The weakest wins, and that is the point rather than a detail: a per-session plugin can have one
// process confined and another not, and a summary that reported the confined one would tell the
// user their plugin is contained while a process of it is not. Reporting nothing for an empty set
// is deliberate too — a plugin that is not running has no mode, and guessing what it would get on a
// future start is a different question from what this answers.
func WeakestSandboxMode(modes []SandboxMode) SandboxMode {
	if len(modes) == 0 {
		return ""
	}
	weakest := modes[0]
	for _, m := range modes[1:] {
		if sandboxStrength(m) < sandboxStrength(weakest) {
			weakest = m
		}
	}
	return weakest
}
