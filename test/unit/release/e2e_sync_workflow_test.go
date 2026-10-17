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

// The sync server ships with every release and every nightly: publish waits for its archives, and
// both workflows package it with the same program, so they cannot ship it differently.
func TestReleaseAndNightlyShipTheSyncServer(t *testing.T) {
	for name, w := range map[string]string{"release.yml": readReleaseWorkflow(t), "nightly.yml": readNightlyWorkflow(t)} {
		w = withoutComments(w)
		if !regexp.MustCompile(`(?m)^\s+needs: \[[^\]]*\bxqsyncd\b[^\]]*\]$`).MatchString(jobSection(t, w, "publish")) {
			t.Errorf("%s: publish does not wait for the xqsyncd archives", name)
		}
		if !strings.Contains(jobSection(t, w, "xqsyncd"), "go run ./scripts/xqsyncdpack") {
			t.Errorf("%s: the server is not packaged by scripts/xqsyncdpack", name)
		}
	}
}

// The nightly fuzz job names its targets by hand, and `go test -fuzz` with a name that matches
// nothing passes without fuzzing anything. Every listed target must exist in the package named
// beside it, and every sync decoder's fuzz target must be listed.
func TestNightlyFuzzesEverySyncDecoder(t *testing.T) {
	job := jobSection(t, readNightlyWorkflow(t), "fuzz")
	listed := regexp.MustCompile(`(?m)^\s+(internal/[\w/]+) (Fuzz\w+)$`).FindAllStringSubmatch(job, -1)
	if len(listed) == 0 {
		t.Fatal("the nightly fuzz job lists no targets")
	}
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range listed {
		pkg, name := m[1], m[2]
		seen[name] = true
		files, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pkg), "*_test.go"))
		found := false
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), "func "+name+"(") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lists %s, which %s does not define; that run would fuzz nothing", "nightly.yml", name, pkg)
		}
	}
	for _, pkg := range []string{"internal/domain/plugin", "internal/domain/synctrust", "internal/domain/syncwire",
		"internal/domain/vaultsync", "internal/infra/opaque", "internal/infra/synccrypto"} {
		files, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pkg), "*_test.go"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			for _, m := range regexp.MustCompile(`func (Fuzz\w+)\(`).FindAllStringSubmatch(string(b), -1) {
				if !seen[m[1]] {
					t.Errorf("%s in %s is not in the nightly fuzz job", m[1], pkg)
				}
			}
		}
	}
}
