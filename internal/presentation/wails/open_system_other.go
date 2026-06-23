//go:build !windows

package wails

import (
	"os/exec"
	"runtime"
)

func openWithSystemDefault(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
