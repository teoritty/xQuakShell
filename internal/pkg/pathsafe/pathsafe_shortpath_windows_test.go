//go:build windows

package pathsafe_test

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"xquakshell/internal/pkg/pathsafe"
)

// TestAnEightDotThreeRootStillMatchesItsLongResolvedForm is the Windows form of the bug that made
// the plugin filesystem boundary refuse legitimate access, and it is the exact shape CI hit.
//
// SecurePathUnderRoots resolves the final path with EvalSymlinks, which returns the canonical long
// name, and then asked whether the RAW root prefixed it. A root handed in as
// C:\Users\RUNNER~1\AppData\Local\Temp\... never prefixes C:\Users\runneradmin\AppData\..., so the
// answer was no and the caller was told "path access denied" for a file inside its own directory.
// Around thirty tests failed that way the first time the suite ran on a Windows runner - and every
// user whose account name is long enough to get an 8.3 alias has the same TEMP path.
//
// The short name is asked of Windows rather than constructed, because 8.3 generation can be turned
// off per volume: where it is off, GetShortPathName returns the long name and there is nothing here
// to test.
func TestAnEightDotThreeRootStillMatchesItsLongResolvedForm(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "a-directory-with-a-long-name")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	wanted := filepath.Join(root, "data.txt")
	if err := os.WriteFile(wanted, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	short := shortPath(t, root)
	if short == root {
		t.Skipf("this volume has no 8.3 alias for %s, so the short-name boundary stays unverified "+
			"here; it is what the Windows CI runner exercises", root)
	}

	got, err := pathsafe.SecurePathUnderRoots(filepath.Join(short, "data.txt"), []string{short})
	if err != nil {
		t.Fatalf("SecurePathUnderRoots refused data.txt inside its own root %s: %v; the short and "+
			"long spellings name the same directory", short, err)
	}
	// Compared in canonical form on both sides. t.TempDir() itself returns the short spelling on a
	// machine whose account name is long, so a raw string comparison here failed on the CI runner
	// while passing everywhere else - the same class of mistake this test exists to catch.
	if got != resolve(t, wanted) {
		t.Errorf("resolved to %q, want %q", got, resolve(t, wanted))
	}
}

// TestAnOpenedFileIsVerifiedAgainstAShortSpeltRoot covers the second place the mismatch bites, and
// the one every plugin filesystem call goes through.
//
// OpenExistingFile re-validates the descriptor after opening it: the path comes back from
// GetFinalPathNameByHandle, which always answers in canonical long form, and is then checked
// against the configured roots. With a short-spelt root nothing matched and every read and write a
// plugin attempted inside its own data directory returned "plugin capability denied".
func TestAnOpenedFileIsVerifiedAgainstAShortSpeltRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "a-directory-with-a-long-name")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	short := shortPath(t, root)
	if short == root {
		t.Skipf("this volume has no 8.3 alias for %s, so the short-name boundary stays unverified here", root)
	}

	body, err := pathsafe.ReadExistingFile([]string{short}, filepath.Join(short, "data.txt"), 1024)
	if err != nil {
		t.Fatalf("ReadExistingFile refused a file inside its own root: %v", err)
	}
	if string(body) != "payload" {
		t.Errorf("read %q, want %q", body, "payload")
	}

	if err := pathsafe.WriteExistingFile([]string{short}, filepath.Join(short, "new.txt"), []byte("x"), 0o600); err != nil {
		t.Errorf("WriteExistingFile refused to create a file inside its own root: %v", err)
	}
}

func resolve(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return filepath.Clean(resolved)
}

func shortPath(t *testing.T, path string) string {
	t.Helper()
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(wide, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("GetShortPathName(%s) failed (%v), so the short-name boundary stays unverified here", path, err)
	}
	return windows.UTF16ToString(buf[:n])
}
