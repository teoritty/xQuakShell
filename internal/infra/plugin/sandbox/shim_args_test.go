package sandbox_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
	"xquakshell/internal/pkg/pathsafe"
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

// FuzzParseShimArgs states the parser's guarantee as a property instead of a list of the escapes
// somebody thought of.
//
// The tests above each name one attack. This is the argv of a process that is about to confine
// itself and then exec an untrusted binary, so the interesting inputs are the ones nobody wrote a
// case for: a flag repeated, a flag whose value is another flag, a value that is empty, a path made
// only of separators. Two invariants have to survive all of them — an accepted instruction never
// carries a writable grant the checker did not vouch for, and a rejected one is always ErrShimArgs,
// because the shim's single response to a bad instruction is to exit without exec'ing anything and
// an error it cannot recognise would take a different path out.
//
// argv[0] is always supplied, as the operating system always supplies it. Fuzzing an empty argv
// would only discover that args[1:] panics on a slice production cannot receive.
func FuzzParseShimArgs(f *testing.F) {
	valid := validShimArgs(f.TempDir())
	f.Add(strings.Join(sandbox.EncodeShimArgs(valid), "\n"))
	f.Add(strings.Join([]string{
		sandbox.FlagShim,
		"--sandbox-data-root=" + valid.DataRoot,
		"--sandbox-allow-rx=" + valid.AllowRX[0],
		"--sandbox-allow-rw=" + filepath.Join(valid.DataRoot, "..", "..", ".ssh"),
		"--sandbox-exec=" + valid.Exec,
	}, "\n"))
	f.Add(sandbox.FlagShim)
	f.Add("")
	f.Add("--sandbox-data-root=\n--sandbox-exec=--sandbox-allow-rw=\n--sandbox-allow-rx=")
	f.Add("--sandbox-data-root=/\n--sandbox-allow-rw=../../../\n--sandbox-allow-rx=/\n--sandbox-exec=/x")

	f.Fuzz(func(t *testing.T, raw string) {
		argv := append([]string{"xQuakShell"}, strings.Split(raw, "\n")...)

		args, err := sandbox.ParseShimArgs(argv)
		if err != nil {
			if !errors.Is(err, sandbox.ErrShimArgs) {
				t.Fatalf("ParseShimArgs(%q) rejected with %v, which is not %v; the shim recognises "+
					"one sentinel and treats anything else as its own bug", raw, err, sandbox.ErrShimArgs)
			}
			return
		}

		switch {
		case args.DataRoot == "":
			t.Errorf("accepted %q with no data root; nothing would bound the writable grants", raw)
		case args.Exec == "":
			t.Errorf("accepted %q with nothing to exec", raw)
		case len(args.AllowRX) == 0:
			t.Errorf("accepted %q with no readable install path", raw)
		}

		for _, rw := range args.AllowRW {
			// Re-vouching for what came back must be a no-op. Anything else means the parser
			// returned a path other than the one it checked — the gap a caller could not see,
			// since the ruleset is opened against what the parser returned.
			again, err := pathsafe.ResolveUnderRoot(args.DataRoot, rw)
			if err != nil {
				t.Errorf("accepted %q, but its writable grant %q does not resolve under %q: %v",
					raw, rw, args.DataRoot, err)
				continue
			}
			if again != rw {
				t.Errorf("accepted %q with writable grant %q, which resolves to %q; the ruleset "+
					"would be opened against a path the check did not vouch for", raw, rw, again)
			}
		}
	})
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
