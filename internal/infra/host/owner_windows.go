//go:build windows

package host

import "os"

func fileOwner(info os.FileInfo) string {
	// Windows does not expose Unix-style UID in os.FileInfo.Sys().
	return "—"
}
