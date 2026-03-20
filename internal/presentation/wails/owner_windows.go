//go:build windows

package wails

import "os"

func getLocalFileOwner(info os.FileInfo, _ string) string {
	// Windows does not expose Unix-style UID in os.FileInfo.Sys().
	// Showing "—" to indicate owner is not available on this platform.
	return "—"
}
