//go:build windows

package main

import (
	"math"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// The window calls x/sys does not wrap, taken straight from user32 as startup_failure_windows.go
// does: this runs before the application exists, so it depends on nothing the application builds.
var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetWindow           = user32.NewProc("GetWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

const gwOwner = 4

// EnumWindows reports each window through a callback with one pointer-sized argument of our own.
// Passing a Go pointer through it would need a uintptr-to-pointer conversion vet rightly flags, so
// the search state is package-level and the mutex makes one search at a time own it. The callback
// is created once: Windows callbacks made by syscall.NewCallback are never freed and the runtime
// caps how many a process may make.
var (
	windowSearchMu    sync.Mutex
	windowSearchPID   uint32
	windowSearchFound windows.HWND
	enumWindowsProc   = sync.OnceValue(func() uintptr { return syscall.NewCallback(matchInstanceWindow) })
)

// activateInstanceWindow brings the running instance's main window to the front, restoring it if
// minimised, and reports whether there was one to bring.
//
// A found window counts even when Windows declines the foreground change and only flashes its
// taskbar button: the instance is alive and the user has been pointed at it. Windows grants the
// change here in practice, because this process was just started by the user's own click.
func activateInstanceWindow(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}
	hwnd := findInstanceWindow(uint32(pid))
	if hwnd == 0 {
		return false
	}
	if iconic, _, _ := procIsIconic.Call(uintptr(hwnd)); iconic != 0 {
		_, _, _ = procShowWindow.Call(uintptr(hwnd), windows.SW_RESTORE)
	}
	_, _, _ = procSetForegroundWindow.Call(uintptr(hwnd))
	return true
}

// findInstanceWindow returns the first visible, unowned top-level window of pid, or 0.
//
// Unowned keeps a dialog the instance has open from being taken for its main window; the dialog
// comes forward with its owner anyway. An invisible window means the instance is still starting or
// already closing, and is left for the caller's next poll.
func findInstanceWindow(pid uint32) windows.HWND {
	windowSearchMu.Lock()
	defer windowSearchMu.Unlock()

	windowSearchPID, windowSearchFound = pid, 0
	// EnumWindows returns an error when the callback stops it early, which is how a match ends
	// the walk; the result is read from windowSearchFound, not from that error.
	_ = windows.EnumWindows(enumWindowsProc(), nil)
	return windowSearchFound
}

func matchInstanceWindow(hwnd windows.HWND, _ uintptr) uintptr {
	const keepLooking, stop = 1, 0
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid != windowSearchPID {
		return keepLooking
	}
	if !windows.IsWindowVisible(hwnd) {
		return keepLooking
	}
	if owner, _, _ := procGetWindow.Call(uintptr(hwnd), gwOwner); owner != 0 {
		return keepLooking
	}
	windowSearchFound = hwnd
	return stop
}
