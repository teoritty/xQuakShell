//go:build !windows

package main

// activateInstanceWindow finds nothing outside Windows, so the guard waits out its polls and then
// reports.
//
// Raising another process's window has no portable answer here: X11 and each Wayland compositor
// allow it differently or not at all, and macOS does it through the application bundle rather than
// a PID. The report still goes to stderr, as startup failures do on these platforms.
func activateInstanceWindow(_ int) bool {
	return false
}
