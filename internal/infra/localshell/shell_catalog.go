// Package localshell starts a shell on the machine the application is running on.
//
// It deliberately shares no code with internal/infra/plugin, which spawns processes too. The
// plugin spawner exists to confine an untrusted binary (ADR-018); this package exists to run the
// user's own shell unconfined. Merging them would put one switch between "sandboxed" and "not",
// and the day someone flips it in the wrong direction is the day the plugin sandbox stops being
// one. Duplicating a process-group kill is the cheaper mistake.
package localshell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"xquakshell/internal/domain"
)

var _ domain.ShellCatalog = (*Catalog)(nil)

// knownShell is one entry in the closed set of shells this application will start.
//
// The set is closed on purpose: it is the reason a stored setting cannot name an arbitrary
// program. Adding a shell is a code change that goes through review, not a string a user or a
// vault can supply.
type knownShell struct {
	id     string
	binary string
}

// knownShells is indexed by the ID that gets persisted. IDs are short, stable and hand-written -
// never derived from a discovered path, because a path that changes between machines would make
// a synced setting resolve differently on each of them.
var knownShells = []knownShell{
	{id: "pwsh", binary: "pwsh"},
	{id: "powershell", binary: "powershell"},
	{id: "cmd", binary: "cmd"},
	{id: "bash", binary: "bash"},
	{id: "zsh", binary: "zsh"},
	{id: "fish", binary: "fish"},
	{id: "sh", binary: "sh"},
}

// preferenceByOS lists which shells to offer, best first, per platform. A shell missing from a
// platform's list is never started there even if its binary happens to exist.
var preferenceByOS = map[string][]string{
	"windows": {"pwsh", "powershell", "cmd"},
	"darwin":  {"zsh", "bash", "fish", "sh"},
	"linux":   {"bash", "zsh", "fish", "sh"},
}

// Catalog discovers the shells present on this machine.
//
// lookPath and getenv are fields rather than direct calls so the discovery rules can be tested
// on any platform: what a catalog decides given a particular machine is the interesting part,
// and it cannot be exercised if the answers come from the machine running the test.
type Catalog struct {
	goos     string
	lookPath func(string) (string, error)
	getenv   func(string) string
}

// NewCatalog returns a Catalog that reads the real machine.
func NewCatalog() *Catalog {
	return &Catalog{goos: runtime.GOOS, lookPath: exec.LookPath, getenv: os.Getenv}
}

// List reports the shells that are both offered on this platform and actually installed, best
// first. The user's own $SHELL is promoted to the front when it names a shell in the closed set.
func (c *Catalog) List() []domain.ShellOption {
	var out []domain.ShellOption
	for _, id := range c.order() {
		shell, ok := lookupKnown(id)
		if !ok {
			continue
		}
		if _, err := c.lookPath(shell.binary); err != nil {
			continue
		}
		out = append(out, domain.ShellOption{ID: shell.id, Name: shell.binary})
	}
	return out
}

// resolve turns a persisted ID into an executable path.
//
// It reports failure for an ID that is unknown, not offered on this platform, or no longer
// installed - three different situations with one correct response, which is to let the caller
// fall back rather than to explain the difference to a user who did not ask.
func (c *Catalog) resolve(id string) (path string, opt domain.ShellOption, ok bool) {
	shell, known := lookupKnown(id)
	if !known || !c.offeredHere(id) {
		return "", domain.ShellOption{}, false
	}
	resolved, err := c.lookPath(shell.binary)
	if err != nil {
		return "", domain.ShellOption{}, false
	}
	return resolved, domain.ShellOption{ID: shell.id, Name: shell.binary}, true
}

// resolveDefault picks the best installed shell for this platform.
func (c *Catalog) resolveDefault() (path string, opt domain.ShellOption, ok bool) {
	for _, id := range c.order() {
		if p, o, found := c.resolve(id); found {
			return p, o, true
		}
	}
	return "", domain.ShellOption{}, false
}

// order returns this platform's preference list with the user's $SHELL promoted to the front.
func (c *Catalog) order() []string {
	base := preferenceByOS[c.goos]
	preferred := c.shellFromEnv()
	if preferred == "" {
		return base
	}
	out := []string{preferred}
	for _, id := range base {
		if id != preferred {
			out = append(out, id)
		}
	}
	return out
}

// shellFromEnv reads $SHELL and returns the known ID it names, or "".
//
// Only the basename is looked at, and only to match it against the closed set - the value itself
// is never used as a path. $SHELL is an environment variable, which means anything that can set
// an environment variable could otherwise choose which program this application executes.
func (c *Catalog) shellFromEnv() string {
	raw := strings.TrimSpace(c.getenv("SHELL"))
	if raw == "" {
		return ""
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(raw)), ".exe")
	for _, shell := range knownShells {
		if shell.id == base && c.offeredHere(shell.id) {
			return shell.id
		}
	}
	return ""
}

func (c *Catalog) offeredHere(id string) bool {
	return slices.Contains(preferenceByOS[c.goos], id)
}

func lookupKnown(id string) (knownShell, bool) {
	for _, shell := range knownShells {
		if shell.id == id {
			return shell, true
		}
	}
	return knownShell{}, false
}
