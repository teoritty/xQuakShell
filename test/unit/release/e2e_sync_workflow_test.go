package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readE2ESyncWorkflow(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e-sync.yml"))
	if err != nil {
		t.Fatalf("e2e-sync.yml: %v", err)
	}
	return string(raw)
}

// The two service-manager jobs run one after the other, never side by side (GitHub volume), and the
// workflow is never on a schedule: it runs when the code it exercises changes, or by hand.
func TestE2ESyncRunsItsJobsInSequenceAndOnlyWhenNeeded(t *testing.T) {
	w := readE2ESyncWorkflow(t)
	if !regexp.MustCompile(`(?m)^\s+needs: linux$`).MatchString(jobSection(t, w, "windows")) {
		t.Error("the windows job must wait for the linux job")
	}
	if strings.Contains(withoutComments(w), "schedule:") {
		t.Error("e2e-sync must not run on a schedule")
	}
	for _, path := range []string{"cmd/xqsyncd/**", "internal/syncserver/**", "packaging/linux/xqsyncd.service", "test/e2e/sync/**"} {
		if !strings.Contains(w, "'"+path+"'") {
			t.Errorf("the path filter misses %s; a change there would not exercise the services", path)
		}
	}
}

// The unit is held to a fixed exposure score, the service directory's ACL is asserted, and both
// installs run the suite against the service rather than a plain process.
func TestE2ESyncChecksTheServicesThemselves(t *testing.T) {
	w := withoutComments(readE2ESyncWorkflow(t))
	linux, windows := jobSection(t, w, "linux"), jobSection(t, w, "windows")
	if !regexp.MustCompile(`systemd-analyze security --threshold=\d`).MatchString(linux) {
		t.Error("the linux job does not hold the unit to an exposure threshold (spec 9.5)")
	}
	if !strings.Contains(windows, "AreAccessRulesProtected") || !strings.Contains(windows, `NT SERVICE\xqsyncd-e2e`) {
		t.Error("the windows job does not assert the data directory ACL (spec 9.5)")
	}
	for name, job := range map[string]string{"linux": linux, "windows": windows} {
		if !strings.Contains(job, "E2E_SYNC_URL") || !strings.Contains(job, "-tags e2e_sync") {
			t.Errorf("the %s job does not run the suite against the installed service", name)
		}
	}
}
