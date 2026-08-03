//go:build !windows

package logwindow

import "os/exec"

func assignProcessToJob(_ *exec.Cmd) {}

func closeJob() {}
