package release_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// findRepoRoot walks up from the test's working directory to the directory holding go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// wailsConfig is the subset of wails.json this package asserts on.
type wailsConfig struct {
	PreBuildHooks map[string]string `json:"preBuildHooks"`
}

func readWailsConfig(t *testing.T) wailsConfig {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "wails.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg wailsConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("wails.json is not valid JSON: %v", err)
	}
	return cfg
}

// A hook that shells out to PowerShell cannot run on a Linux or macOS build host. Wails matches
// hook keys as "<platform>/<arch>", so such a hook must be scoped to the windows platform.
func TestPowerShellPreBuildHooksAreWindowsScoped(t *testing.T) {
	cfg := readWailsConfig(t)
	if len(cfg.PreBuildHooks) == 0 {
		t.Fatal("wails.json declares no preBuildHooks; expected the icon-copy hook")
	}
	for key, command := range cfg.PreBuildHooks {
		if !strings.Contains(strings.ToLower(command), "powershell") {
			continue
		}
		if !strings.HasPrefix(key, "windows/") {
			t.Errorf("preBuildHooks key %q runs PowerShell but is not scoped to windows/*; "+
				"it would run and fail on a Linux build host", key)
		}
	}
}

// The icon-copy hook must still exist for Windows builds, otherwise build/windows/icon.ico
// is missing and the executable loses its icon.
func TestWindowsIconHookStillPresent(t *testing.T) {
	cfg := readWailsConfig(t)
	for key, command := range cfg.PreBuildHooks {
		if strings.HasPrefix(key, "windows/") && strings.Contains(command, "icon.ico") {
			return
		}
	}
	t.Error("no windows/* preBuildHook copies icon.ico; the Windows build would lose its icon")
}

func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// An unpinned CLI means two runs of the same tag can produce different binaries. Release builds
// must be reproducible, so the Wails CLI version is pinned and kept equal to the library version
// in go.mod.
func TestWailsCLIIsPinnedToTheGoModVersion(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	libVersion := regexp.MustCompile(`github\.com/wailsapp/wails/v2 (v[\d.]+)`).FindSubmatch(goMod)
	if libVersion == nil {
		t.Fatal("could not find the wails/v2 version in go.mod")
	}

	workflow := readReleaseWorkflow(t)
	if strings.Contains(workflow, "wails@latest") {
		t.Error("release.yml installs wails@latest; release builds must pin the CLI version")
	}
	want := "cmd/wails@" + string(libVersion[1])
	if !strings.Contains(workflow, want) {
		t.Errorf("release.yml does not install %q; the CLI must match the library version in go.mod", want)
	}

	// Contains() above is satisfied by one correct pin, and release.yml carries two - one per
	// build job. A bump that updated only the windows one would pass on the strength of that
	// single match while the linux archive was built by a different CLI than the windows one,
	// which is precisely the irreproducibility the pin exists to prevent. So every pin in the
	// file has to agree, not merely one of them.
	for _, pin := range regexp.MustCompile(`cmd/wails@v[\d.]+`).FindAllString(workflow, -1) {
		if pin != want {
			t.Errorf("release.yml pins %q as well as %q; every job must install the same CLI", pin, want)
		}
	}
}

// The Makefile's WAILS_VERSION is the version a developer is told to install when the CLI is
// missing, so a stale one sends them to a build that does not match CI's. It drifts for the same
// reason the release.yml pins do: Dependabot edits go.mod and nothing else.
func TestMakefileWailsVersionMatchesGoMod(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	libVersion := regexp.MustCompile(`github\.com/wailsapp/wails/v2 (v[\d.]+)`).FindSubmatch(goMod)
	if libVersion == nil {
		t.Fatal("could not find the wails/v2 version in go.mod")
	}

	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	declared := regexp.MustCompile(`(?m)^WAILS_VERSION\s*:=\s*(v[\d.]+)`).FindSubmatch(makefile)
	if declared == nil {
		t.Fatal("the Makefile no longer declares WAILS_VERSION; the require-wails guard depends on it")
	}
	if got, want := string(declared[1]), string(libVersion[1]); got != want {
		t.Errorf("Makefile WAILS_VERSION = %s, go.mod requires %s; a developer following the "+
			"install hint would get a different CLI than CI uses", got, want)
	}
}

// The release must stamp the tag into the binary, using the exact symbol path proven to work by
// TestAppVersionIsOverridableByLdflags.
func TestReleaseWorkflowStampsTheAppVersion(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	const symbol = "xquakshell/internal/presentation/wails.AppVersion"
	if !strings.Contains(workflow, "-X "+symbol+"=") {
		t.Errorf("release.yml does not pass -ldflags \"-X %s=<version>\"; "+
			"released binaries would report the fallback version", symbol)
	}
}

// SHA256SUMS is the integrity control for the release. It must be generated from the artifacts
// that are actually published, in the same job that publishes them — not from a build job that
// only sees its own platform's files.
func TestChecksumsCoverEveryPublishedArchive(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	if !strings.Contains(workflow, "SHA256SUMS") {
		t.Fatal("release.yml no longer produces SHA256SUMS")
	}
	if !strings.Contains(workflow, "actions/download-artifact") {
		t.Error("the publish job does not download build artifacts; " +
			"SHA256SUMS cannot cover archives built in other jobs")
	}
}

// A tag carrying a pre-release suffix (v1.0.0-rc.1) must publish as a GitHub pre-release, so an
// rc is never presented as a stable download.
func TestPrereleaseDetectionPreserved(t *testing.T) {
	workflow := readReleaseWorkflow(t)
	if !strings.Contains(workflow, "contains(github.ref_name, '-')") {
		t.Error("release.yml lost the pre-release detection rule; " +
			"an rc tag would publish as a stable release")
	}
}
