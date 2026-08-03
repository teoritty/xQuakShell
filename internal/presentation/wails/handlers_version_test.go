package wails_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/presentation/wails"
)

// ldflagsSymbol is the fully qualified linker symbol the release workflow overrides. It is
// duplicated here deliberately: if the package ever moves, this test fails and points at every
// other place that must be updated.
const ldflagsSymbol = "xquakshell/internal/presentation/wails.AppVersion"

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
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

// The default compiled-in version must look like a release version, so a dev build never reports
// something empty or obviously wrong in the About panel.
func TestAppVersionDefaultIsSemver(t *testing.T) {
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	if !semver.MatchString(wails.AppVersion) {
		t.Errorf("AppVersion = %q, want a semver string like 1.0.0 or 1.0.0-rc.1", wails.AppVersion)
	}
}

// This is the load-bearing test. It links the probe fixture with the same -X flag the release
// workflow uses and asserts the override reached the binary. A typo in the symbol path makes the
// linker silently keep the default, which would ship an rc binary reporting the wrong version.
func TestAppVersionIsOverridableByLdflags(t *testing.T) {
	const want = "9.9.9-ldflags-probe"

	cmd := exec.Command("go", "run",
		"-ldflags", "-X "+ldflagsSymbol+"="+want,
		"./test/fixtures/versionprobe")
	cmd.Dir = repoRoot(t)

	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go run versionprobe: %v\n%s", err, stderr)
	}

	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("linked AppVersion = %q, want %q; the -X symbol path %q did not take effect",
			got, want, ldflagsSymbol)
	}
}

// GetVersionInfo must surface all three versions, each from its own source of truth. They are
// distinct concepts (ADR-012) and must not collapse into one value.
func TestGetVersionInfoReportsAllThreeVersions(t *testing.T) {
	api := &wails.AppAPI{}

	got := api.GetVersionInfo()

	if got.AppVersion != wails.AppVersion {
		t.Errorf("AppVersion = %q, want %q", got.AppVersion, wails.AppVersion)
	}
	if got.CoreVersion != domainplugin.HostCoreVersion {
		t.Errorf("CoreVersion = %q, want %q", got.CoreVersion, domainplugin.HostCoreVersion)
	}
	if got.PluginAPIVersion != domainplugin.PluginAPIVersion {
		t.Errorf("PluginAPIVersion = %q, want %q", got.PluginAPIVersion, domainplugin.PluginAPIVersion)
	}
}
