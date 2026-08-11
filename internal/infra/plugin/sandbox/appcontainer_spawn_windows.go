//go:build windows

package sandbox

import (
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"xquakshell/internal/pkg/safego"
)

// procThreadAttributeSecurityCapabilities is PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES, which
// x/sys does not define. It is ProcThreadAttributeValue(9, FALSE, TRUE, FALSE) — number 9, marked
// as a thread-inheritable input.
const procThreadAttributeSecurityCapabilities = 0x00020009

// stillActive is STILL_ACTIVE, the exit code GetExitCodeProcess reports for a process that has not
// exited. x/sys does not define it.
const stillActive = 259

// securityCapabilities is SECURITY_CAPABILITIES. Capabilities is left nil and CapabilityCount zero:
// a capability is what re-opens part of the world to an AppContainer, and this design grants none.
// In particular there is no internetClient, which is what denies the plugin every socket — outbound,
// inbound and loopback alike.
type securityCapabilities struct {
	AppContainerSid *windows.SID
	Capabilities    *windows.SIDAndAttributes
	CapabilityCount uint32
	Reserved        uint32
}

// Process is a plugin process running inside an AppContainer.
//
// It exists because os/exec cannot produce one: syscall.SysProcAttr offers Token and no attribute
// list, so PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES cannot be passed and the child has to be
// built by hand from pipes and a STARTUPINFOEX. Pid, Wait and Kill are shaped to satisfy the
// caller's childProcess seam without it having to know which of the two spawns produced this.
type Process struct {
	handle windows.Handle
	pid    int

	Stdin  io.WriteCloser
	Stdout io.ReadCloser

	waitOnce sync.Once
	waitErr  error
	closed   sync.Once
}

// SpawnRequest is everything CreateProcessW needs that this package cannot work out for itself.
type SpawnRequest struct {
	// Exe is the plugin binary. It is passed as lpApplicationName as well as argv[0], so the
	// command line can never be reinterpreted as naming a different program.
	Exe string
	// Dir is the process's working directory, or "" to inherit the host's — which is what every
	// plugin gets, and deliberately. A directory of its own would have to be one the container can
	// traverse, and traverse on a directory is the same right as execute on a file, so granting it
	// on the only writable directory a plugin has would hand it "write a binary, then run it". An
	// inherited directory it cannot read costs a plugin nothing: it is given every path it needs
	// absolutely, at initialize.
	Dir string
	// Env is the child's whole environment. It must contain LOCALAPPDATA — see PluginContainerEnv.
	Env []string
	// Stderr receives the child's standard error. The caller owns it and closes it.
	Stderr io.Writer
}

// Spawn starts the process inside the container and returns it with its pipes attached.
func Spawn(c *Container, req SpawnRequest) (*Process, error) {
	pipes, err := newStdioPipes()
	if err != nil {
		return nil, err
	}
	defer pipes.closeChildEnds()

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		pipes.closeParentEnds()
		return nil, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	defer attrs.Delete()

	caps := securityCapabilities{AppContainerSid: c.sid}
	if err := attrs.Update(procThreadAttributeSecurityCapabilities,
		unsafe.Pointer(&caps), unsafe.Sizeof(caps)); err != nil {
		pipes.closeParentEnds()
		return nil, fmt.Errorf("UpdateProcThreadAttribute(SECURITY_CAPABILITIES): %w", err)
	}

	pi, err := createProcess(req, pipes, attrs)
	if err != nil {
		pipes.closeParentEnds()
		return nil, err
	}
	_ = windows.CloseHandle(pi.Thread)

	proc := &Process{
		handle: pi.Process,
		pid:    int(pi.ProcessId),
		Stdin:  os.NewFile(uintptr(pipes.stdinWrite), "plugin-stdin"),
		Stdout: os.NewFile(uintptr(pipes.stdoutRead), "plugin-stdout"),
	}
	stderrPipe := os.NewFile(uintptr(pipes.stderrRead), "plugin-stderr")
	safego.GoNamed("plugin.containerStderr", func() { pumpStderr(stderrPipe, req.Stderr) })
	return proc, nil
}

func createProcess(req SpawnRequest, pipes *stdioPipes, attrs *windows.ProcThreadAttributeListContainer) (windows.ProcessInformation, error) {
	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput = pipes.stdinRead
	si.StdOutput = pipes.stdoutWrite
	si.StdErr = pipes.stderrWrite
	si.ProcThreadAttributeList = attrs.List()

	exePtr, err := windows.UTF16PtrFromString(req.Exe)
	if err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("plugin path: %w", err)
	}
	// argv is the executable and nothing else. A plugin takes no arguments, and naming the
	// executable in both lpApplicationName and argv[0] means the command line cannot be read as
	// selecting a different program however the path is spelled.
	cmdPtr, err := windows.UTF16PtrFromString(windows.EscapeArg(req.Exe))
	if err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("plugin command line: %w", err)
	}
	var dirPtr *uint16
	if req.Dir != "" {
		if dirPtr, err = windows.UTF16PtrFromString(req.Dir); err != nil {
			return windows.ProcessInformation{}, fmt.Errorf("plugin working directory: %w", err)
		}
	}
	envBlock, err := environmentBlock(req.Env)
	if err != nil {
		return windows.ProcessInformation{}, err
	}

	var pi windows.ProcessInformation
	if err := windows.CreateProcess(exePtr, cmdPtr, nil, nil, true,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW,
		&envBlock[0], dirPtr, &si.StartupInfo, &pi); err != nil {
		return windows.ProcessInformation{}, fmt.Errorf("CreateProcess into app container: %w", err)
	}
	return pi, nil
}

func (p *Process) Pid() int { return p.pid }

// Wait blocks until the process exits and reports a non-zero exit as an error, matching what
// exec.Cmd.Wait gives the reaper for the other spawn.
func (p *Process) Wait() error {
	p.waitOnce.Do(func() {
		if _, err := windows.WaitForSingleObject(p.handle, windows.INFINITE); err != nil {
			p.waitErr = fmt.Errorf("wait for plugin process: %w", err)
			return
		}
		var code uint32
		if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
			p.waitErr = fmt.Errorf("exit code of plugin process: %w", err)
			return
		}
		if code != 0 {
			p.waitErr = fmt.Errorf("plugin process exited with code %d", code)
		}
		p.close()
	})
	return p.waitErr
}

// Kill terminates this process and nothing else, matching the seam's contract. The job object the
// host also assigns is what takes down anything the process managed to start.
func (p *Process) Kill() error {
	if err := windows.TerminateProcess(p.handle, 1); err != nil {
		// A process that has already exited is not a failed kill; it is the outcome asked for.
		if code, codeErr := exitCode(p.handle); codeErr == nil && code != stillActive {
			return nil
		}
		return fmt.Errorf("terminate plugin process: %w", err)
	}
	return nil
}

func (p *Process) close() {
	p.closed.Do(func() { _ = windows.CloseHandle(p.handle) })
}

func exitCode(handle windows.Handle) (uint32, error) {
	var code uint32
	err := windows.GetExitCodeProcess(handle, &code)
	return code, err
}

// pumpStderr copies the child's stderr into the host's redacting writer. It owns the read end and
// closes it, so the goroutine ends when the child does.
func pumpStderr(from *os.File, to io.Writer) {
	defer func() { _ = from.Close() }()
	if to == nil {
		_, _ = io.Copy(io.Discard, from)
		return
	}
	_, _ = io.Copy(to, from)
}

// environmentBlock renders a UTF-16, double-NUL-terminated environment block.
func environmentBlock(env []string) ([]uint16, error) {
	block := make([]uint16, 0, 256)
	for _, entry := range env {
		encoded, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, fmt.Errorf("plugin environment entry: %w", err)
		}
		block = append(block, encoded...)
	}
	// The extra NUL terminates the block. An empty environment still needs two, which is why the
	// slice is never handed over empty.
	return append(block, 0), nil
}
