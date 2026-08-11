package plugin

import "errors"

// SandboxMode reports what OS-level boundary a plugin process is actually running behind.
//
// It describes an outcome, never an intention. A process that reports one of the two enforced modes
// is one the operating system is holding inside its own directories; a process that reports either
// of the others has the user's full reach, and the capability gate — which is an IPC boundary, not a
// syscall one — is all that stands between the plugin and the rest of the machine.
type SandboxMode string

const (
	// SandboxEnforced means the OS is confining this process along every dimension this design
	// claims: it can reach its own directories and nothing else, and it can open no socket.
	SandboxEnforced SandboxMode = "enforced"

	// SandboxEnforcedPartial means the OS is confining this process along some of those dimensions
	// and not all of them.
	//
	// It exists so that a partial guarantee is never rounded up to a whole one in what the user is
	// told. A Linux kernel between 5.13 and 6.7 has Landlock filesystem rules and no network rules,
	// so the plugin is held inside its own directories and can still dial anywhere; nothing in the
	// plugin contract needs a socket, so this denies the plugin nothing it was allowed to do, but
	// "sandboxed" and "sandboxed except for the network" are different sentences and the user gets
	// the true one. SandboxSupport.Reason names what is missing.
	SandboxEnforcedPartial SandboxMode = "enforced-partial"

	// SandboxUnavailable means this platform, kernel or build cannot confine it. This is not a
	// failure and does not stop a plugin from starting: it is the honest state of macOS, of a Linux
	// kernel without Landlock, and of every platform in builds where the isolation is not yet
	// implemented.
	SandboxUnavailable SandboxMode = "unavailable"

	// SandboxDisabled means the platform can confine the process and the user chose not to.
	SandboxDisabled SandboxMode = "disabled"
)

// SandboxPolicy is what the host has decided to do about confinement, handed to the layer that
// creates processes.
//
// It carries a decision and not a setting, which is the whole reason it exists as a type. Whether a
// user may run a plugin unconfined is a policy question; infra knows only two facts — can this
// platform confine, and did this attempt succeed — and must not learn which setting governs the
// answer or where that setting is stored.
type SandboxPolicy struct {
	// AllowUnsandboxedFallback permits a start to continue when the platform can confine a plugin
	// and the attempt failed. It never applies to a platform that cannot confine at all: that case
	// is not a failure and starts unconfined with no opt-in.
	AllowUnsandboxedFallback bool
}

// ErrSandboxUnavailable reports that a platform which can confine a plugin failed to.
//
// It is a distinct error because the two cases must never share a code path. "This platform has no
// sandbox" and "this platform has one and it did not work" look identical from the outside and call
// for opposite responses — start, and refuse — and collapsing them is exactly how a sandbox stops
// working for a fraction of users with nobody noticing.
var ErrSandboxUnavailable = errors.New("the plugin sandbox could not be applied")

// SandboxSupport is what a platform can do, asked once per build and independent of any plugin.
type SandboxSupport struct {
	// Available reports whether this build on this machine can confine a plugin process at all.
	Available bool

	// Network reports whether that confinement covers sockets as well as files. It is meaningful
	// only when Available is true, and it is a second flag rather than a degree of the first
	// because Linux delivers the two independently: Landlock gained filesystem rules in kernel
	// 5.13 and network rules only in 6.7, so a kernel in between confines every path a plugin can
	// reach and none of the addresses it can dial.
	Network bool

	// Reason says what is not covered, for the log and the UI: why nothing is confined when
	// Available is false, and which dimension is missing when it is true but incomplete. It is the
	// whole value of this type — "unavailable" without a reason sends a user looking through
	// settings for a switch that does not exist.
	Reason string
}

// Mode is the mode a process gets on a platform with this support and nothing else deciding.
func (s SandboxSupport) Mode() SandboxMode {
	switch {
	case !s.Available:
		return SandboxUnavailable
	case !s.Network:
		return SandboxEnforcedPartial
	default:
		return SandboxEnforced
	}
}

// sandboxStrength orders the modes so that the weakest one can be picked out of a set. Only the two
// enforced modes confine anything, and a partial one ranks below a whole one because that is the
// direction a summary must round; the bottom two confine nothing and differ only in why.
func sandboxStrength(m SandboxMode) int {
	switch m {
	case SandboxEnforced:
		return 3
	case SandboxEnforcedPartial:
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
