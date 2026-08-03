package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Windows archives are what a user actually receives: a folder they unpack and run from, with
// no installer and nothing to consult but what is inside it. These assertions cover the shape of
// that folder and the one check that keeps the runtime-carrying archive from shipping a runtime
// that does not work.

// windowsJob returns the portion of release.yml belonging to the windows job.
func windowsJob(t *testing.T) string {
	t.Helper()
	workflow := readReleaseWorkflow(t)
	start := strings.Index(workflow, "\n  windows:")
	if start < 0 {
		t.Fatal("release.yml has no windows job")
	}
	rest := workflow[start+1:]
	if end := regexp.MustCompile(`\n  [a-z][\w-]*:`).FindStringIndex(rest); end != nil {
		return rest[:end[0]]
	}
	return rest
}

// The archive with the runtime exists for exactly one reason: to start on a machine that has none.
// If the file Wails loads is not in it, the archive is not merely incomplete — it is worse than the
// smaller one, because a present-but-unusable runtime is fatal where an absent one still lets Wails
// offer to install it. The release stops rather than shipping that.
func TestWebView2ArchiveIsVerifiedBeforeItIsPublished(t *testing.T) {
	job := windowsJob(t)

	if !strings.Contains(job, "EmbeddedBrowserWebView.dll") {
		t.Error("the packaging step never checks that the bundled runtime contains the file Wails " +
			"loads; a broken runtime would be published as the archive that fixes broken runtimes")
	}
	if !strings.Contains(job, "throw") {
		t.Error("the check does not fail the job, so a broken archive would still be uploaded")
	}
}

// Both archives unpack into a folder of their own. The app keeps its vault, audit log and plugins
// in data\ beside the executable, so the unpacked folder is the installation; a zip that spills an
// exe and a 250 MB runtime folder into the user's Downloads separates them at the first tidy-up.
func TestWindowsArchivesUnpackIntoTheirOwnFolder(t *testing.T) {
	job := windowsJob(t)

	// Both archives are built the same way, so both Compress-Archive calls must zip a staging
	// folder rather than loose files.
	calls := regexp.MustCompile(`Compress-Archive -Path "([^"]+)"`).FindAllStringSubmatch(job, -1)
	if len(calls) != 2 {
		t.Fatalf("expected the two Windows archives to be zipped, found %d Compress-Archive calls", len(calls))
	}
	for _, call := range calls {
		source := call[1]
		if strings.HasSuffix(source, "*") || !strings.HasPrefix(source, "stage/") {
			t.Errorf("Compress-Archive zips %q; zipping anything but a single staging folder puts "+
				"the contents at the root of the archive, with no directory of their own", source)
		}
	}
}

// A portable archive is the only documentation its user has: they double-click an unsigned binary
// on Windows, meet SmartScreen, and have to decide what to do with no context at all.
func TestWindowsArchivesCarryTheirDocumentation(t *testing.T) {
	job := windowsJob(t)

	for _, path := range []string{"packaging/windows/README.md", "LICENSE"} {
		if strings.Count(job, path) < 2 {
			t.Errorf("%s is not copied into both Windows archives", path)
		}
		root, err := findRepoRoot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("%s is copied into the archive but does not exist: %v", path, err)
		}
	}

	readme, err := os.ReadFile(filepath.Join(mustRepoRoot(t), "packaging", "windows", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"SmartScreen", "SHA256SUMS", "WebView2"} {
		if !strings.Contains(string(readme), topic) {
			t.Errorf("the Windows archive README never mentions %s, which is one of the three "+
				"things a first-time user runs into", topic)
		}
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
