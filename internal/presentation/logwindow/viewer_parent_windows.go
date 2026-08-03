//go:build windows

package logwindow

import (
	"golang.org/x/sys/windows"
)

func waitProcessExit(pid int) bool {
	const synchronize = 0x00100000
	// #nosec G115 -- Windows PIDs are DWORDs; a wrapped value simply fails OpenProcess.
	h, err := windows.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	event, _ := windows.WaitForSingleObject(h, 0)
	return event == windows.WAIT_OBJECT_0
}

func windowsProcessAlive(pid int) bool {
	// #nosec G115 -- Windows PIDs are DWORDs; a wrapped value simply fails OpenProcess.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	err = windows.GetExitCodeProcess(h, &code)
	if err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}
