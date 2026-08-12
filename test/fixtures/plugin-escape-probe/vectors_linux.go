//go:build linux

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"xquakshell/test/fixtures/escapeprobe"
)

// platformVectors are the escapes only Linux offers. /proc is the interesting one: it is a
// filesystem, so it is Landlock's job, and nothing grants it — which means a plugin that can read
// /proc/<host>/environ has walked out through a door the filesystem rules were supposed to hold.
func platformVectors() map[string]func(escapeprobe.Target) error {
	return map[string]func(escapeprobe.Target) error{
		escapeprobe.VectorProcEnviron:  readHostEnviron,
		escapeprobe.VectorProcSelfRoot: readThroughProcSelfRoot,
		escapeprobe.VectorPtraceHost:   attachToHost,
	}
}

func readHostEnviron(target escapeprobe.Target) error {
	_, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", target.Pid))
	return err
}

// readThroughProcSelfRoot takes the classic route around a path-based sandbox: /proc/self/root is a
// magic symlink to /, so prefixing it produces a second name for a file the plugin was refused
// under its own name.
func readThroughProcSelfRoot(target escapeprobe.Target) error {
	_, err := os.ReadFile("/proc/self/root" + target.Path)
	return err
}

// attachToHost tries to become the tracer of a process outside the sandbox. Success would make
// every filesystem rule beside the point: the plugin would be reading another process's memory.
//
// The attach has to happen on a locked thread and be undone on that same thread — ptrace requests
// are per-thread, and a tracee whose tracer wandered off is left stopped.
func attachToHost(target escapeprobe.Target) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := syscall.PtraceAttach(target.Pid); err != nil {
		return err
	}
	var status syscall.WaitStatus
	if _, err := syscall.Wait4(target.Pid, &status, 0, nil); err != nil {
		return fmt.Errorf("attached to %d but could not wait for it: %w", target.Pid, err)
	}
	if err := syscall.PtraceDetach(target.Pid); err != nil {
		return fmt.Errorf("attached to %d and could not detach: %w", target.Pid, err)
	}
	return nil
}
