package localshell

import "testing"

// fakeMachine describes a pretend computer: which binaries are on PATH and what the environment
// says. Discovery rules are the thing under test, and they cannot be exercised against the real
// machine - the answers there depend on whoever is running the suite.
type fakeMachine struct {
	goos      string
	installed map[string]string
	env       map[string]string
}

func (m fakeMachine) catalog() *Catalog {
	return &Catalog{
		goos: m.goos,
		lookPath: func(name string) (string, error) {
			if path, ok := m.installed[name]; ok {
				return path, nil
			}
			return "", errNotInstalled
		},
		getenv: func(key string) string { return m.env[key] },
	}
}

var errNotInstalled = &notInstalledError{}

type notInstalledError struct{}

func (*notInstalledError) Error() string { return "not installed" }

func TestListOffersOnlyInstalledShells(t *testing.T) {
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash", "sh": "/bin/sh"},
	}.catalog()

	got := c.List()
	if len(got) != 2 {
		t.Fatalf("List() returned %d shells, want 2; zsh and fish are not installed", len(got))
	}
	if got[0].ID != "bash" || got[1].ID != "sh" {
		t.Errorf("List() = %v, want bash then sh in platform preference order", got)
	}
}

func TestListExcludesShellsNotOfferedOnThisPlatform(t *testing.T) {
	// cmd.exe under WSL or a cross-mounted PATH is findable on Linux. It is still not something
	// this application starts there, and the closed per-platform list is what stops it.
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash", "cmd": "/mnt/c/Windows/System32/cmd.exe"},
	}.catalog()

	for _, opt := range c.List() {
		if opt.ID == "cmd" {
			t.Fatalf("List() offered cmd on linux; the per-platform list must exclude it")
		}
	}
}

func TestShellEnvPromotesUserChoiceToFront(t *testing.T) {
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash", "zsh": "/usr/bin/zsh"},
		env:       map[string]string{"SHELL": "/usr/bin/zsh"},
	}.catalog()

	got := c.List()
	if len(got) == 0 || got[0].ID != "zsh" {
		t.Errorf("List() = %v, want zsh first; $SHELL names the user's own choice", got)
	}
}

func TestShellEnvNeverBecomesAPath(t *testing.T) {
	// The whole point of matching $SHELL by basename against the closed set: an environment
	// variable is settable by anything in the process tree, so its value must never reach a
	// process launcher. A hostile $SHELL resolves to nothing and the default is used.
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash"},
		env:       map[string]string{"SHELL": "/tmp/evil/payload"},
	}.catalog()

	if got := c.shellFromEnv(); got != "" {
		t.Errorf("shellFromEnv() = %q, want empty; an unknown basename must not be honoured", got)
	}
	path, opt, ok := c.resolveDefault()
	if !ok || opt.ID != "bash" || path != "/bin/bash" {
		t.Errorf("resolveDefault() = %q/%v/%v, want /bin/bash and bash", path, opt, ok)
	}
}

func TestShellEnvNamingAnUninstalledShellIsIgnored(t *testing.T) {
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash"},
		env:       map[string]string{"SHELL": "/usr/bin/fish"},
	}.catalog()

	_, opt, ok := c.resolveDefault()
	if !ok || opt.ID != "bash" {
		t.Errorf("resolveDefault() picked %v, want bash; fish is named but not installed", opt)
	}
}

func TestResolveRejectsUnknownID(t *testing.T) {
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash", "python3": "/usr/bin/python3"},
	}.catalog()

	if _, _, ok := c.resolve("python3"); ok {
		t.Error("resolve(python3) succeeded; only IDs in the closed set may resolve")
	}
	if _, _, ok := c.resolve("../../bin/sh"); ok {
		t.Error("resolve() honoured a path-shaped id; ids are tokens, never paths")
	}
}

func TestResolveRejectsShellNotOfferedOnThisPlatform(t *testing.T) {
	// The path that matters for a setting carried between machines: a vault written on Windows
	// says "cmd", and the same vault is opened on Linux where a cross-mounted cmd.exe is on PATH.
	// List never offers it there, but resolve is reached with the stored id directly and is the
	// only thing standing between that id and an exec.
	c := fakeMachine{
		goos:      "linux",
		installed: map[string]string{"bash": "/bin/bash", "cmd": "/mnt/c/Windows/System32/cmd.exe"},
	}.catalog()

	if _, _, ok := c.resolve("cmd"); ok {
		t.Fatal("resolve(cmd) succeeded on linux; a known id must still be refused off its platform")
	}
}

func TestResolveRejectsUninstalledShell(t *testing.T) {
	// The user picked pwsh and later uninstalled it. resolve must say no so the caller falls
	// back, rather than handing out a path that no longer runs.
	c := fakeMachine{
		goos:      "windows",
		installed: map[string]string{"powershell": `C:\Windows\System32\powershell.exe`},
	}.catalog()

	if _, _, ok := c.resolve("pwsh"); ok {
		t.Error("resolve(pwsh) succeeded while pwsh is not installed")
	}
	_, opt, ok := c.resolveDefault()
	if !ok || opt.ID != "powershell" {
		t.Errorf("resolveDefault() = %v, want powershell as the next best on windows", opt)
	}
}

func TestResolveDefaultReportsFailureOnABareMachine(t *testing.T) {
	c := fakeMachine{goos: "linux"}.catalog()

	if _, _, ok := c.resolveDefault(); ok {
		t.Error("resolveDefault() succeeded with nothing installed")
	}
}
