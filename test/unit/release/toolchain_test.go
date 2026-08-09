package release_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// go.mod names the compiler the module requires; every workflow names the compiler CI installs.
// Nothing keeps the two together on its own. Dependabot's gomod ecosystem updates module
// requirements, not the `go` directive, and go-version is a step input rather than an action
// version, so its github-actions ecosystem leaves that alone too — both are manual, and they had
// silently drifted a whole major apart before this test existed: the module asked for 1.25.12 while
// development ran on 1.26.5, so "it builds here" was a statement about a compiler CI never used.

var goVersionSpec = regexp.MustCompile(`go-version: '([^']+)'`)

// setupGo counts the steps that install a toolchain, so a step added without a version cannot slip
// past: setup-go with no spec silently takes whatever the runner image happens to ship.
var setupGo = regexp.MustCompile(`uses: actions/setup-go@`)

func TestWorkflowGoVersionsMatchGoMod(t *testing.T) {
	root := mustRepoRoot(t)

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?m)^go (\d+\.\d+)(?:\.\d+)?`).FindSubmatch(goMod)
	if declared == nil {
		t.Fatal("go.mod has no go directive; there is nothing to hold the workflows against")
	}
	// The workflows pin a minor and let the runner resolve the newest patch of it, which is what
	// keeps a patch release from needing eight edits. So the comparison is on major.minor.
	want := string(declared[1])

	files, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no workflows found; this check is checking nothing")
	}

	pins := 0
	for _, file := range files {
		pins += checkWorkflowGoVersion(t, file, want)
	}
	if pins == 0 {
		t.Fatal("no go-version pin found in any workflow; either CI stopped installing Go or the " +
			"pattern this test looks for has changed and it has stopped checking anything")
	}
}

// checkWorkflowGoVersion reports how many pins it examined so the caller can refuse a run that
// examined none.
func checkWorkflowGoVersion(t *testing.T, file, want string) int {
	t.Helper()

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	name := filepath.Base(file)

	specs := goVersionSpec.FindAllStringSubmatch(body, -1)
	// go-version-file: go.mod is the other correct answer — it cannot drift by construction — so it
	// counts as a pin rather than as a missing one.
	fromFile := strings.Count(body, "go-version-file:")

	if installs := len(setupGo.FindAllString(body, -1)); installs != len(specs)+fromFile {
		t.Errorf("%s sets up Go %d times but names a version %d times; a setup-go step without one "+
			"builds with whatever the runner image ships", name, installs, len(specs)+fromFile)
	}

	for _, spec := range specs {
		if err := versionSatisfies(spec[1], want); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	return len(specs) + fromFile
}

// versionSatisfies accepts a pin naming the same major.minor as go.mod, at either precision: '1.26'
// resolves to the newest 1.26.x, and '1.26.5' is exact. A pin on any other minor is the drift this
// test exists to catch.
func versionSatisfies(spec, want string) error {
	if spec == want || strings.HasPrefix(spec, want+".") {
		return nil
	}
	return fmt.Errorf("go-version: '%s' does not match the %s required by go.mod; CI would build "+
		"the module with a compiler it does not ask for", spec, want)
}
