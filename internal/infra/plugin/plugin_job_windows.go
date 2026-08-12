//go:build windows

package plugin

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	domainplugin "xquakshell/internal/domain/plugin"
)

func createPluginJob() (pluginJob, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return pluginJob{}, fmt.Errorf("CreateJobObject: %w", err)
	}

	if err := applyJobResourceLimits(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return pluginJob{}, err
	}
	if err := applyJobUIRestrictions(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return pluginJob{}, err
	}

	return pluginJob{handle: uintptr(handle)}, nil
}

func applyJobResourceLimits(handle windows.Handle) error {
	mem := domainplugin.MaxPluginProcessMemoryBytes
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags =
		windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
			windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
			windows.JOB_OBJECT_LIMIT_JOB_MEMORY |
			windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	info.BasicLimitInformation.ActiveProcessLimit = 1
	info.ProcessMemoryLimit = uintptr(mem)
	info.JobMemoryLimit = uintptr(mem)

	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return fmt.Errorf("SetInformationJobObject (limits): %w", err)
	}
	return nil
}

// applyJobUIRestrictions denies the plugin process the parts of the USER/GDI surface that let one
// process of a user reach into another's session state. It is not a sandbox and does not pretend
// to be one: the plugin still runs under the user's own token and can read the user's files. What
// it removes is the cheap, no-privilege-needed half of that reach — reading the clipboard a user
// just pasted a password into, enumerating and driving other applications' windows, or switching
// the desktop out from under them.
//
// Every flag Windows offers is set, because a headless JSON-RPC process needs none of them. Plugin
// UI is drawn by the host from IPC data or served as assets into the host WebView (ADR-008); a
// plugin that creates a window of its own is already outside the contract. GLOBALATOMS is the only
// one with a plausible false positive — it gives the job a private atom table, which would break a
// plugin coordinating with another process through a shared global atom — and that is a thing this
// design has no legitimate reason to do.
//
// ActiveProcessLimit is 1, so there are no descendants to reason about: the restriction applies to
// the one process in the job.
func applyJobUIRestrictions(handle windows.Handle) error {
	info := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{
		UIRestrictionsClass: windows.JOB_OBJECT_UILIMIT_HANDLES |
			windows.JOB_OBJECT_UILIMIT_READCLIPBOARD |
			windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD |
			windows.JOB_OBJECT_UILIMIT_DESKTOP |
			windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS |
			windows.JOB_OBJECT_UILIMIT_EXITWINDOWS |
			windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS |
			windows.JOB_OBJECT_UILIMIT_GLOBALATOMS,
	}

	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectBasicUIRestrictions,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return fmt.Errorf("SetInformationJobObject (UI restrictions): %w", err)
	}
	return nil
}

func assignProcessToJob(job pluginJob, pid int) error {
	if job.handle == 0 || pid <= 0 {
		return fmt.Errorf("plugin job object unavailable")
	}
	// #nosec G115 -- pid > 0 is checked above and Windows PIDs are DWORDs.
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open plugin process for job assignment: %w", err)
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(windows.Handle(job.handle), ph); err != nil {
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}

func closePluginJob(job pluginJob) {
	if job.handle != 0 {
		_ = windows.CloseHandle(windows.Handle(job.handle))
	}
}

func applyPluginResourceLimits(_ int, job pluginJob) error {
	if job.handle == 0 {
		return fmt.Errorf("plugin job object unavailable")
	}
	return nil
}
