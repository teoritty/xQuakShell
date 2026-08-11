package sandbox_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
)

// shimArgv builds the argv a launched shim would see: a program name the parser must skip, then
// the encoded instruction.
func shimArgv(a sandbox.ShimArgs) []string {
	return append([]string{"xQuakShell"}, sandbox.EncodeShimArgs(a)...)
}

func validShimArgs(root string) sandbox.ShimArgs {
	return sandbox.ShimArgs{
		DataRoot: filepath.Join(root, "plugins"),
		AllowRX: []string{
			filepath.Join(root, "plugins", "vnc", "bin"),
			filepath.Join(root, "plugins", "vnc", "plugin.json"),
		},
		AllowRW: []string{filepath.Join(root, "plugins", "vnc", "data", "sess-1")},
		Exec:    filepath.Join(root, "plugins", "vnc", "bin", "plugin"),
	}
}

func TestShimArgsSurviveTheArgvRoundTrip(t *testing.T) {
	want := validShimArgs(t.TempDir())

	got, err := sandbox.ParseShimArgs(shimArgv(want))
	if err != nil {
		t.Fatalf("ParseShimArgs of an instruction this package encoded: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the instruction:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseShimArgsRefusesAWritableGrantOutsideTheDataRoot(t *testing.T) {
	root := t.TempDir()
	args := validShimArgs(root)
	// The shape that matters: a relative escape rather than an obviously foreign absolute path,
	// because that is what a mistake looks like when the host builds the argv by joining.
	args.AllowRW = []string{filepath.Join(root, "plugins", "..", "..", ".ssh")}

	_, err := sandbox.ParseShimArgs(shimArgv(args))
	if !errors.Is(err, sandbox.ErrShimArgs) {
		t.Fatalf("ParseShimArgs error = %v, want %v; a writable grant outside the plugin data "+
			"root turns the shim into a way to hand a plugin an arbitrary directory",
			err, sandbox.ErrShimArgs)
	}
}

func TestParseShimArgsKeepsAWritableGrantThatOnlyLooksLikeAnEscape(t *testing.T) {
	root := t.TempDir()
	args := validShimArgs(root)
	inside := filepath.Join(root, "plugins", "vnc", "data", "..", "data", "sess-1")
	args.AllowRW = []string{inside}

	got, err := sandbox.ParseShimArgs(shimArgv(args))
	if err != nil {
		t.Fatalf("ParseShimArgs rejected a path that stays inside the root: %v", err)
	}
	want := filepath.Join(root, "plugins", "vnc", "data", "sess-1")
	if len(got.AllowRW) != 1 || got.AllowRW[0] != want {
		t.Errorf("AllowRW = %v, want [%s]; the parser returns the resolved path so the ruleset "+
			"opens the directory the check actually vouched for", got.AllowRW, want)
	}
}

func TestParseShimArgsRefusesAnExecutableNoGrantCovers(t *testing.T) {
	root := t.TempDir()
	args := validShimArgs(root)
	args.Exec = filepath.Join(root, "plugins", "other", "bin", "plugin")

	_, err := sandbox.ParseShimArgs(shimArgv(args))
	if !errors.Is(err, sandbox.ErrShimArgs) {
		t.Fatalf("ParseShimArgs error = %v, want %v; a binary the ruleset does not cover dies at "+
			"execve with nothing to explain it", err, sandbox.ErrShimArgs)
	}
}

func TestParseShimArgsRefusesAnIncompleteInstruction(t *testing.T) {
	root := t.TempDir()
	cases := map[string]func(*sandbox.ShimArgs){
		"no data root":  func(a *sandbox.ShimArgs) { a.DataRoot = "" },
		"no executable": func(a *sandbox.ShimArgs) { a.Exec = "" },
		"no install path": func(a *sandbox.ShimArgs) {
			a.AllowRX = nil
			a.Exec = filepath.Join(root, "plugins", "vnc", "bin", "plugin")
		},
	}

	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			args := validShimArgs(root)
			break_(&args)
			if _, err := sandbox.ParseShimArgs(shimArgv(args)); !errors.Is(err, sandbox.ErrShimArgs) {
				t.Errorf("ParseShimArgs error = %v, want %v", err, sandbox.ErrShimArgs)
			}
		})
	}
}

func TestIsShimModeReadsOnlyTheFlagAndNeverTheProgramName(t *testing.T) {
	if sandbox.IsShimMode([]string{"xQuakShell"}) {
		t.Error("IsShimMode = true for a plain launch; the application would never start")
	}
	if !sandbox.IsShimMode([]string{"xQuakShell", sandbox.FlagShim}) {
		t.Error("IsShimMode = false with the flag present; the plugin would launch unconfined")
	}
	// argv[0] is attacker-adjacent in a way the flags are not: it is whatever the launching
	// process chose to call this binary.
	if sandbox.IsShimMode([]string{sandbox.FlagShim}) {
		t.Errorf("IsShimMode = true for %q as the program name; the flag must be an argument",
			sandbox.FlagShim)
	}
}
