package main

import (
	"errors"
	"log/slog"
	"time"

	"xquakshell/internal/infra/portable"
)

const (
	alreadyRunningTitle = "xQuakShell is already running"
	alreadyRunningBody  = "" +
		"Another xQuakShell window is already open with the data in this folder.\n\n" +
		"Two copies working on the same vault would overwrite each other's changes, so this one " +
		"has closed. Switch to the open window, or close it and start xQuakShell again."

	// exitAlreadyRunning is the status when this process gave way without finding the other
	// window to bring forward. Finding it is a success - the user asked for xQuakShell and got it -
	// and exits 0.
	exitAlreadyRunning = 1

	// Five seconds covers the instance the user just closed and immediately reopened: the old
	// process keeps the claim while it stops plugins and flushes the vault, after its window is
	// already gone. It is also long enough for a second double-click to find the first launch's
	// window once that window exists. Past that, a real second instance is waiting on nothing.
	instanceClaimPolls    = 20
	instanceClaimInterval = 250 * time.Millisecond
)

// instanceLock is what a successful claim holds until the process has finished with the data root.
type instanceLock interface {
	Release() error
}

// instanceGuard decides whether this process may use a data root. Every effect is a field so the
// decision can be tested without a second process, a window or a dialog.
type instanceGuard struct {
	acquire  func(dataRoot string) (instanceLock, error)
	owner    func(dataRoot string) (int, bool)
	activate func(pid int) bool
	wait     func()
	report   func(title, body string)
	polls    int
}

// instanceClaim is the guard's verdict.
//
// degraded is set when the claim could not be attempted and the process starts unprotected; it is
// returned rather than logged because the guard runs before composeApp installs the log hub, and a
// warning written then would reach no log a user can open.
type instanceClaim struct {
	proceed  bool
	exitCode int
	release  func()
	degraded error
}

func defaultInstanceGuard() instanceGuard {
	return instanceGuard{
		acquire: func(dataRoot string) (instanceLock, error) {
			return portable.AcquireInstanceLock(dataRoot)
		},
		owner:    portable.InstanceLockOwner,
		activate: activateInstanceWindow,
		wait:     func() { time.Sleep(instanceClaimInterval) },
		report:   showStartupFailure,
		polls:    instanceClaimPolls,
	}
}

// claim takes the data root for this process or explains why it cannot.
//
// A failure to lock that is not another instance holding the lock - a read-only stick, a
// filesystem without lock support - lets the process start as every build before this one did. A
// read-only data root cannot be written by either instance, and refusing to start on unusual media
// would trade a narrow risk for a certain outage.
func (g instanceGuard) claim(dataRoot string) instanceClaim {
	for attempt := 0; ; attempt++ {
		lock, err := g.acquire(dataRoot)
		if err == nil {
			return instanceClaim{proceed: true, release: releaseClaim(lock)}
		}
		if !errors.Is(err, portable.ErrInstanceRunning) {
			return instanceClaim{proceed: true, release: func() {}, degraded: err}
		}
		if pid, ok := g.owner(dataRoot); ok && g.activate(pid) {
			return instanceClaim{exitCode: 0}
		}
		if attempt >= g.polls {
			break
		}
		g.wait()
	}
	g.report(alreadyRunningTitle, alreadyRunningBody)
	return instanceClaim{exitCode: exitAlreadyRunning}
}

// releaseClaim drops the claim. A failure is only logged: this runs as the process exits, after the
// vault has been flushed, and the kernel releases the lock with the process regardless.
func releaseClaim(lock instanceLock) func() {
	return func() {
		if err := lock.Release(); err != nil {
			slog.Warn("instance lock release failed", "err", err)
		}
	}
}
