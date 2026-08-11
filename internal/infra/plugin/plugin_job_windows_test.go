//go:build windows

package plugin

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	domainplugin "xquakshell/internal/domain/plugin"
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

func TestCreatePluginJobSetsUIRestrictions(t *testing.T) {
	job, err := createPluginJob()
	if err != nil {
		t.Fatalf("createPluginJob: %v", err)
	}
	defer closePluginJob(job)

	var info windows.JOBOBJECT_BASIC_UI_RESTRICTIONS
	var returned uint32
	if err := windows.QueryInformationJobObject(
		windows.Handle(job.handle),
		windows.JobObjectBasicUIRestrictions,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		&returned,
	); err != nil {
		t.Fatalf("query job UI restrictions: %v", err)
	}

	// Named individually so a failure says which reach was left open, not just that a bitmask
	// differs. Each line is a thing a plugin process could otherwise do to the user's session.
	restrictions := []struct {
		flag uint32
		what string
	}{
		{windows.JOB_OBJECT_UILIMIT_HANDLES, "use USER handles created outside the job (drive other applications' windows)"},
		{windows.JOB_OBJECT_UILIMIT_READCLIPBOARD, "read the clipboard"},
		{windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD, "write the clipboard"},
		{windows.JOB_OBJECT_UILIMIT_DESKTOP, "create or switch desktops"},
		{windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS, "change display settings"},
		{windows.JOB_OBJECT_UILIMIT_EXITWINDOWS, "log the user off or shut the machine down"},
		{windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS, "change system parameters"},
		{windows.JOB_OBJECT_UILIMIT_GLOBALATOMS, "reach the global atom table"},
	}
	for _, r := range restrictions {
		if info.UIRestrictionsClass&r.flag == 0 {
			t.Errorf("UI restriction 0x%08x not set: a plugin process could still %s", r.flag, r.what)
		}
	}
}

func TestJobObjectAvailable(t *testing.T) {
	if !JobObjectAvailable() {
		t.Fatal("expected job object available")
	}
}
