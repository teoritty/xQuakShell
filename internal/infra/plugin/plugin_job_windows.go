//go:build windows

package plugin

import (
	"fmt"
	"unsafe"

	domainplugin "ssh-client/internal/domain/plugin"
	"golang.org/x/sys/windows"
)

func createPluginJob() (pluginJob, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return pluginJob{}, fmt.Errorf("CreateJobObject: %w", err)
	}

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
		_ = windows.CloseHandle(handle)
		return pluginJob{}, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	return pluginJob{handle: uintptr(handle)}, nil
}

func assignProcessToJob(job pluginJob, pid int) error {
	if job.handle == 0 || pid <= 0 {
		return fmt.Errorf("plugin job object unavailable")
	}
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
