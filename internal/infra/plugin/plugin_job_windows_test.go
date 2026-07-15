//go:build windows

package plugin

import (
	"testing"
	"unsafe"

	domainplugin "ssh-client/internal/domain/plugin"
	"golang.org/x/sys/windows"
)

func TestCreatePluginJobSetsMemoryLimits(t *testing.T) {
	job, err := createPluginJob()
	if err != nil {
		t.Fatalf("createPluginJob: %v", err)
	}
	defer closePluginJob(job)

	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var returned uint32
	err = windows.QueryInformationJobObject(
		windows.Handle(job.handle),
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		&returned,
	)
	if err != nil {
		t.Fatalf("query job object: %v", err)
	}
	if info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatalf("kill-on-close not set")
	}
	if info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY == 0 {
		t.Fatalf("process memory limit not set")
	}
	if info.ProcessMemoryLimit != uintptr(domainplugin.MaxPluginProcessMemoryBytes) {
		t.Fatalf("process memory limit = %d, want %d", info.ProcessMemoryLimit, domainplugin.MaxPluginProcessMemoryBytes)
	}
}

func TestJobObjectAvailable(t *testing.T) {
	if !JobObjectAvailable() {
		t.Fatal("expected job object available")
	}
}
