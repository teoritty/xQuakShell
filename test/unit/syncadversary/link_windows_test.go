//go:build windows

package syncadversary

import (
	"os/exec"
	"testing"
)

// plantDirLink plants an NTFS junction, which unlike a symlink needs no privilege to create.
func plantDirLink(t *testing.T, target, link string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %v: %s", err, out)
	}
}
