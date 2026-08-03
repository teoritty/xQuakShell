package release_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
