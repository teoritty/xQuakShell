//go:build windows

package localshell

import (
	"log/slog"
	"math"
	"syscall"
	"unsafe"

	"github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/windows"
)

// startupArgs makes a shell speak UTF-8 before the user sees it.
//
// A fresh console starts on the system's OEM code page, while the terminal renderer on the other
// end decodes UTF-8 - so on any non-English Windows the first command that prints a non-ASCII
// filename comes back as mojibake. Fixing it from the host side is not on offer: the code page
// belongs to the console the child attaches to, and only the child can change it once attached.
//
// PowerShell 7 already defaults to UTF-8, so it gets nothing. The redirect on the others
// suppresses the "Active code page: 65001" line that would otherwise be the first thing in every
// new terminal.
func startupArgs(shellID string) []string {
	switch shellID {
	case "cmd":
		return []string{"/K", "chcp 65001>nul"}
	case "powershell":
		// The spaces around the redirect are load-bearing. PowerShell's parser does not split
		// `65001>$null` into an argument and a redirect, so chcp receives `65001>` and answers
		// "Invalid parameter format" into the terminal the user just opened.
		return []string{
			"-NoExit", "-Command",
			"chcp 65001 > $null; " +
				"[Console]::OutputEncoding = [Console]::InputEncoding = [Text.UTF8Encoding]::new($false)",
		}
	default:
		return nil
	}
}

// configureCmd hides the console window Windows would otherwise flash up for the child. The
// pseudo-console is the real terminal; a second window appearing for an instant is pure noise.
func configureCmd(cmd *pty.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

// processGroup owns the shell and everything it starts.
type processGroup struct {
	handle windows.Handle
}

// adopt puts the shell into a job object whose only limit is that closing it kills the job.
//
// Deliberately nothing like the plugin job object, which caps memory, forbids descendants
// entirely (ActiveProcessLimit 1) and strips the UI surface. Those exist to confine an untrusted
// binary. A shell exists to start other programs, so the only thing wanted here is the guarantee
// that closing the tab takes the whole tree with it - the `ping -t` left running, the build that
// was still going - rather than leaving orphans behind with nothing on screen to stop them.
func (t *terminal) adopt() {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}

	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		slog.Warn("local terminal: no job object, children may outlive the tab", "err", err)
		return
	}

	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(handle)
		slog.Warn("local terminal: job object limits rejected", "err", err)
		return
	}

	// A Windows process id is a DWORD, but os.Process carries it as an int. The range check is
	// not defensive noise: an unchecked narrowing conversion here would be a silent wrap, and a
	// wrapped id names some other process for OpenProcess to hand us a handle to.
	pid := t.cmd.Process.Pid
	if pid <= 0 || pid > math.MaxUint32 {
		_ = windows.CloseHandle(handle)
		slog.Warn("local terminal: shell process id out of range", "pid", pid)
		return
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		_ = windows.CloseHandle(handle)
		slog.Warn("local terminal: cannot open shell process", "err", err)
		return
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(handle, proc); err != nil {
		_ = windows.CloseHandle(handle)
		slog.Warn("local terminal: cannot assign shell to job", "err", err)
		return
	}
	t.job.handle = handle
}

// killTree closes the job handle, which terminates every process still inside it.
func (t *terminal) killTree() {
	if t.job.handle != 0 {
		if err := windows.CloseHandle(t.job.handle); err != nil {
			slog.Warn("local terminal: closing job handle failed", "err", err)
		}
		t.job.handle = 0
		return
	}
	// No job object was established, so the shell is all that can be reached. Its children, if
	// any, are already orphaned and there is nothing here that can find them.
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
}
