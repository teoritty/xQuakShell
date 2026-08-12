//go:build windows

package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	domainplugin "xquakshell/internal/domain/plugin"
)

// TestTheDataDirectoryIsExcludedEvenWhenTheDataRootIsSpeltShort is the cross-session isolation
// guard, asked at the spelling that defeated it.
//
// installReadPaths resolves the install root through EvalSymlinks and then compared the entries it
// found against a data directory named from the RAW data root. On Windows those are routinely two
// different strings for one directory: a profile with a long account name gets an 8.3 alias, so
// C:\Users\RUNNER~1\... never matched C:\Users\runneradmin\.... The exclusion matched nothing, the
// data directory was granted read access along with the rest of the install tree, and an
// inheritable ACE there reaches every session's instance directory - which is the precise defect
// the entry-by-entry enumeration was written to prevent.
func TestTheDataDirectoryIsExcludedEvenWhenTheDataRootIsSpeltShort(t *testing.T) {
	longRoot := t.TempDir()
	pluginID := "com.example.read-paths-probe"
	installDir := filepath.Join(PluginsRoot(longRoot), pluginID)
	for _, dir := range []string{
		filepath.Join(installDir, "bin"),
		filepath.Join(installDir, "data", "sess-a"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	shortRoot := shortPathOf(t, longRoot)
	if shortRoot == longRoot {
		t.Skipf("this volume has no 8.3 alias for %s, so the short-name spelling stays unverified "+
			"here; the Windows CI runner is where it bites", longRoot)
	}

	plugin := domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: pluginID},
		RootDir:  filepath.Join(PluginsRoot(shortRoot), pluginID),
	}
	granted, err := installReadPaths(shortRoot, plugin)
	if err != nil {
		t.Fatalf("installReadPaths: %v", err)
	}

	for _, path := range granted {
		if filepath.Base(path) == "data" {
			t.Errorf("installReadPaths granted %s; a read grant on the data directory carries an "+
				"inheritable ACE onto every session's instance directory", path)
		}
	}
	if len(granted) == 0 {
		t.Error("installReadPaths granted nothing; the plugin could not read its own binary")
	}
}

func shortPathOf(t *testing.T, path string) string {
	t.Helper()
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(wide, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("GetShortPathName(%s) failed (%v)", path, err)
	}
	return windows.UTF16ToString(buf[:n])
}
