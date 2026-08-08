package docs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A link checker proves a document points at something that exists. It cannot prove the document
// tells the truth, and every version in a README is a hand-copied claim about a file somewhere else
// — which is how this repository came to advertise Node 18 long after Vite stopped starting on it,
// and to tell contributors to install the Wails CLI from @latest while three other places pinned it.
//
// These tests read the source of truth and require the prose to agree. They are the doc equivalent
// of TestMakefileWailsVersionMatchesGoMod, which has kept the Makefile honest the same way.
//
// Each one fails loudly when it cannot find the value it compares against, rather than passing on
// an empty match: a check that silently stops checking is worse than no check, because it also
// stops anyone from noticing.

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root above the test's working directory")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.Join(parts...)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// capture returns the first submatch, failing the test when the pattern no longer matches. The
// failure names the file, because a pattern that stops matching means the source of truth moved and
// the test has to follow it — not that the claim under test is fine.
func capture(t *testing.T, pattern, text, source string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("%s no longer matches %q; this check has stopped checking anything", source, pattern)
	}
	return m[1]
}

// The README states the Go version twice - once in a badge, once as a prerequisite - and go.mod is
// the only place it is enforced. A reader installing the version the badge names has to end up with
// a toolchain that can build the module.
func TestReadmeGoVersionMatchesGoMod(t *testing.T) {
	want := capture(t, `(?m)^go (\d+\.\d+(?:\.\d+)?)`, readRepoFile(t, "go.mod"), "go.mod")
	readme := readRepoFile(t, "README.md")

	for _, claim := range []string{"go-" + want + "-", "Go " + want + "+"} {
		if !strings.Contains(readme, claim) {
			t.Errorf("README.md does not contain %q; go.mod requires Go %s", claim, want)
		}
	}
}

// The CLI version is written in five places: go.mod, the Makefile, both build workflows and the
// README. The first four already agree by test; the README was the one nobody was checking, and it
// said @latest. Two builds of one tag have to agree, and that starts with the reader's toolchain.
func TestReadmeWailsPinMatchesGoMod(t *testing.T) {
	want := capture(t, `github\.com/wailsapp/wails/v2 (v[\d.]+)`, readRepoFile(t, "go.mod"), "go.mod")
	readme := readRepoFile(t, "README.md")

	if strings.Contains(readme, "wails@latest") {
		t.Error("README.md tells the reader to install wails@latest; the CLI is pinned everywhere " +
			"else, and an unpinned CLI makes two builds of one tag disagree")
	}
	if !strings.Contains(readme, "Wails-"+want+"-") {
		t.Errorf("the README badge does not name Wails %s, which is the version in go.mod", want)
	}

	pins := regexp.MustCompile(`cmd/wails@v[\d.]+`).FindAllString(readme, -1)
	if len(pins) == 0 {
		t.Fatal("README.md no longer shows how to install the Wails CLI")
	}
	for _, pin := range pins {
		if pin != "cmd/wails@"+want {
			t.Errorf("README.md installs %q; go.mod requires %s", pin, want)
		}
	}
}

// Documented at high while the workflow gates at moderate, this instruction passed locally on
// exactly the findings that fail the pull request - the worst kind of wrong, because following it
// produces confidence rather than an error.
func TestDocumentedNpmAuditLevelMatchesTheWorkflow(t *testing.T) {
	want := capture(t, `npm audit --omit=dev --audit-level=(\w+)`,
		readRepoFile(t, ".github", "workflows", "security.yml"), "security.yml")

	got := capture(t, `npm audit --omit=dev --audit-level=(\w+)`,
		readRepoFile(t, "CONTRIBUTING.md"), "CONTRIBUTING.md")

	if got != want {
		t.Errorf("CONTRIBUTING.md documents --audit-level=%s, the workflow gates at %s; the "+
			"documented command passes on a finding that fails the PR", got, want)
	}
}

// The floor is Vite's, not ours, and it moved under us once already: Vite 8 raised it and the
// README kept saying 18, so following it got you an install that could not run. package-lock.json
// is the committed copy of that requirement - node_modules is not, so the test must not read it.
func TestReadmeNodeFloorMatchesVite(t *testing.T) {
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
			Engines struct {
				Node string `json:"node"`
			} `json:"engines"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend", "package-lock.json")), &lock); err != nil {
		t.Fatal(err)
	}

	vite := lock.Packages["node_modules/vite"]
	engines := vite.Engines.Node
	if engines == "" {
		t.Fatal("package-lock.json records no engines.node for vite; the floor now has to be read " +
			"from somewhere else and this test has to follow it")
	}

	// "^20.19.0 || >=22.12.0" yields 20.19 and 22.12 — the numbers a reader is told to have. The
	// patch level is dropped deliberately: Vite states one, the README does not need to.
	readme := readRepoFile(t, "README.md")
	majorMinor := regexp.MustCompile(`(\d+\.\d+)(?:\.\d+)?`).FindAllStringSubmatch(engines, -1)
	if len(majorMinor) == 0 {
		t.Fatalf("could not read any version out of vite's engines.node = %q", engines)
	}
	for _, version := range majorMinor {
		if !strings.Contains(readme, version[1]) {
			t.Errorf("README.md never mentions Node %s, which vite %s requires (engines.node = %q)",
				version[1], vite.Version, engines)
		}
	}
}
