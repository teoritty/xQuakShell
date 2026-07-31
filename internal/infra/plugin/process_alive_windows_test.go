//go:build windows

package plugin

import "golang.org/x/sys/windows"

// stillActive is Windows' STILL_ACTIVE (259): GetExitCodeProcess reports it for a process that has
// not exited yet.
const stillActive = 259

// processAliveForTest answers whether the OS process is still running.
//
// It asks the kernel rather than the host, because the whole point of the test using it is a process
// the host no longer knows about. A pid alone is not proof on Windows — GetExitCodeProcess on an
// exited process still succeeds while a handle to it exists — so the exit code is what decides.
func processAliveForTest(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
