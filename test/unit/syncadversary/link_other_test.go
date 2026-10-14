//go:build !windows

package syncadversary

import (
	"os"
	"testing"
)

// plantDirLink plants a directory symlink.
func plantDirLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
