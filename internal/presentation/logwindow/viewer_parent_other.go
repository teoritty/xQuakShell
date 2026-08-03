//go:build !windows

package logwindow

func waitProcessExit(_ int) bool { return false }

func windowsProcessAlive(_ int) bool { return true }
