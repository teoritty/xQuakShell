// Package sandbox answers one question per platform: can this build confine a plugin process to
// its own directories, and if not, why not — and on the platforms where it can, applies the
// confinement.
//
// It deliberately holds no policy. What to do about an answer — start anyway, refuse, or let the
// user override — is a decision for the usecase layer, which is where the setting that governs it
// lives. This package reports facts about the operating system and acts on them.
package sandbox

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"xquakshell/internal/pkg/pathsafe"
)

// FlagShim marks a re-execution of this binary as the sandbox shim rather than the application.
//
// A Landlock ruleset is self-applied and survives execve, which is the only reason this shim
// exists: the host cannot restrict a child from the outside, Go offers no hook between fork and
// exec, and an untrusted plugin binary cannot be asked to restrict itself. So the host launches
// itself, the shim confines the process it is already in, and only then execs the plugin into that
// confinement. It follows the log viewer's precedent — one early dispatch in main before anything
// else is built.
const FlagShim = "--plugin-sandbox"

const (
	flagDataRoot = "--sandbox-data-root="
	flagAllowRW  = "--sandbox-allow-rw="
	flagAllowRX  = "--sandbox-allow-rx="
	flagExec     = "--sandbox-exec="
)

// ErrShimArgs reports an instruction the shim refuses to act on. It is one sentinel rather than a
// family because there is exactly one response to any of them: exit without exec'ing anything.
var ErrShimArgs = errors.New("plugin sandbox arguments")

// ShimArgs is the whole instruction the host gives the shim: what the confined process may reach,
// and what to become once it is confined.
//
// Paths are absolute and carry no plugin identity. The shim grants what it is told to grant and
// resolves nothing on its own — deriving a data directory here would put the layout of the plugin
// tree in two places, and the second copy is the one that drifts.
type ShimArgs struct {
	// DataRoot bounds every writable grant. Nothing else in this struct is trusted to stay inside
	// it; that is what makes the check below worth running.
	DataRoot string

	// AllowRW is what the process may read, write and create under — its own instance directory,
	// and nothing shared with another instance.
	AllowRW []string

	// AllowRX is what it may read and execute: its own installed files, one grant per entry so
	// that a data directory nested inside the install tree is not swept in with them.
	AllowRX []string

	// Exec is the plugin binary this process becomes.
	Exec string
}

// IsShimMode reports whether args start the plugin sandbox shim.
func IsShimMode(args []string) bool {
	return slices.Contains(args[1:], FlagShim)
}

// EncodeShimArgs renders the instruction as the argv tail the host appends after its own path.
func EncodeShimArgs(a ShimArgs) []string {
	argv := []string{FlagShim, flagDataRoot + a.DataRoot}
	for _, p := range a.AllowRX {
		argv = append(argv, flagAllowRX+p)
	}
	for _, p := range a.AllowRW {
		argv = append(argv, flagAllowRW+p)
	}
	return append(argv, flagExec+a.Exec)
}

// ParseShimArgs reads the instruction back and refuses one it cannot vouch for.
//
// The host writes this argv and the shim reads it inside the same binary, so the checks here are
// defence in depth rather than a trust boundary. They are worth the few lines anyway: the one
// mistake that would matter — a writable grant that resolves outside the plugin data root — turns
// this shim from a confinement into a way to hand a plugin write access to an arbitrary directory,
// and it would be a silent success rather than a crash.
func ParseShimArgs(args []string) (ShimArgs, error) {
	a := collectShimArgs(args)
	switch {
	case a.DataRoot == "":
		return ShimArgs{}, fmt.Errorf("%w: no data root", ErrShimArgs)
	case a.Exec == "":
		return ShimArgs{}, fmt.Errorf("%w: no executable", ErrShimArgs)
	case len(a.AllowRX) == 0:
		return ShimArgs{}, fmt.Errorf("%w: no readable install path", ErrShimArgs)
	}
	for i, p := range a.AllowRW {
		resolved, err := pathsafe.ResolveUnderRoot(a.DataRoot, p)
		if err != nil {
			return ShimArgs{}, fmt.Errorf("%w: writable grant %q is outside the plugin data root", ErrShimArgs, p)
		}
		a.AllowRW[i] = resolved
	}
	if !underAny(a.AllowRX, a.Exec) {
		return ShimArgs{}, fmt.Errorf("%w: executable %q is outside every readable install path", ErrShimArgs, a.Exec)
	}
	return a, nil
}

func collectShimArgs(args []string) ShimArgs {
	var a ShimArgs
	for _, raw := range args[1:] {
		switch {
		case strings.HasPrefix(raw, flagDataRoot):
			a.DataRoot = strings.TrimPrefix(raw, flagDataRoot)
		case strings.HasPrefix(raw, flagAllowRW):
			a.AllowRW = append(a.AllowRW, strings.TrimPrefix(raw, flagAllowRW))
		case strings.HasPrefix(raw, flagAllowRX):
			a.AllowRX = append(a.AllowRX, strings.TrimPrefix(raw, flagAllowRX))
		case strings.HasPrefix(raw, flagExec):
			a.Exec = strings.TrimPrefix(raw, flagExec)
		}
	}
	return a
}

// underAny reports whether target sits under one of the roots. The executable must, because a
// binary the ruleset does not cover cannot be exec'd into it — an argv that says otherwise
// describes a process that would die at execve with nothing to explain it.
func underAny(roots []string, target string) bool {
	target = filepath.Clean(target)
	for _, root := range roots {
		if pathsafe.UnderRoot(root, target) {
			return true
		}
	}
	return false
}
