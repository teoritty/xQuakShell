package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"xquakshell/internal/infra/portable"
)

type fakeLock struct{ released int }

func (l *fakeLock) Release() error {
	l.released++
	return nil
}

// guardProbe records what the guard did, so each test can assert the one effect it is about and
// also that the effects that must not happen did not.
type guardProbe struct {
	acquireErrs []error // one per attempt; nil means the claim succeeds
	lock        *fakeLock
	ownerPID    int
	activateOK  bool

	attempts  int
	activated []int
	waits     int
	reports   int
}

func (p *guardProbe) guard(polls int) instanceGuard {
	p.lock = &fakeLock{}
	return instanceGuard{
		acquire: func(string) (instanceLock, error) {
			err := p.acquireErrs[min(p.attempts, len(p.acquireErrs)-1)]
			p.attempts++
			if err != nil {
				return nil, err
			}
			return p.lock, nil
		},
		owner: func(string) (int, bool) { return p.ownerPID, p.ownerPID > 0 },
		activate: func(pid int) bool {
			p.activated = append(p.activated, pid)
			return p.activateOK
		},
		wait:   func() { p.waits++ },
		report: func(string, string) { p.reports++ },
		polls:  polls,
	}
}

var errHeld = fmt.Errorf("instance lock data/instance.lock: %w", portable.ErrInstanceRunning)

func TestFreeDataRootIsClaimed(t *testing.T) {
	p := &guardProbe{acquireErrs: []error{nil}}
	claim := p.guard(20).claim("data")

	if !claim.proceed || claim.degraded != nil {
		t.Fatalf("claim = %+v, want proceed without degradation", claim)
	}
	if p.lock.released != 0 {
		t.Fatal("lock released before the application ran")
	}
	claim.release()
	if p.lock.released != 1 {
		t.Errorf("release called %d times, want 1", p.lock.released)
	}
	if p.waits+p.reports+len(p.activated) != 0 {
		t.Errorf("free data root waited %d, reported %d, activated %v; none should happen",
			p.waits, p.reports, p.activated)
	}
}

// The common case of a second launch: the first instance is up, its window comes forward, and the
// second goes away without a dialog - the user asked for xQuakShell and got it.
func TestRunningInstanceIsBroughtForwardSilently(t *testing.T) {
	p := &guardProbe{acquireErrs: []error{errHeld}, ownerPID: 4242, activateOK: true}
	claim := p.guard(20).claim("data")

	if claim.proceed {
		t.Fatal("proceeded while another instance holds the data root")
	}
	if claim.exitCode != 0 {
		t.Errorf("exit code = %d, want 0; finding the running window is a success", claim.exitCode)
	}
	if len(p.activated) != 1 || p.activated[0] != 4242 {
		t.Errorf("activated %v, want exactly the recorded owner 4242", p.activated)
	}
	if p.reports != 0 || p.waits != 0 {
		t.Errorf("reported %d, waited %d; an activated window needs neither", p.reports, p.waits)
	}
}

// The instance that was just closed still holds the claim while it flushes the vault and stops
// plugins, after its window is gone. Reopening in that moment must wait for it, not refuse.
func TestClosingInstanceIsWaitedFor(t *testing.T) {
	p := &guardProbe{acquireErrs: []error{errHeld, errHeld, nil}, ownerPID: 4242}
	claim := p.guard(20).claim("data")

	if !claim.proceed {
		t.Fatalf("claim = %+v, want proceed once the old instance let go", claim)
	}
	if p.waits != 2 || p.reports != 0 {
		t.Errorf("waited %d, reported %d; want 2 waits and no report", p.waits, p.reports)
	}
}

// Double-clicking twice: the first launch holds the claim before its window exists. The second
// has to keep looking for that window rather than give up on the first miss.
func TestWindowAppearingLaterIsStillFound(t *testing.T) {
	p := &guardProbe{acquireErrs: []error{errHeld}, ownerPID: 4242}
	guard := p.guard(20)
	base := guard.activate
	guard.activate = func(pid int) bool {
		base(pid)
		return len(p.activated) == 3
	}

	claim := guard.claim("data")

	if claim.proceed || claim.exitCode != 0 || p.reports != 0 {
		t.Errorf("claim = %+v, reports %d; want a silent exit once the window appeared", claim, p.reports)
	}
	if p.waits != 2 {
		t.Errorf("waited %d times, want 2 before the third look found the window", p.waits)
	}
}

func TestInstanceThatNeverLetsGoIsReported(t *testing.T) {
	const polls = 5
	p := &guardProbe{acquireErrs: []error{errHeld}}
	claim := p.guard(polls).claim("data")

	if claim.proceed {
		t.Fatal("proceeded while another instance holds the data root")
	}
	if claim.exitCode != exitAlreadyRunning {
		t.Errorf("exit code = %d, want %d", claim.exitCode, exitAlreadyRunning)
	}
	if p.reports != 1 {
		t.Errorf("reported %d times, want 1; silence here looks like a launch that did nothing", p.reports)
	}
	if p.waits != polls || p.attempts != polls+1 {
		t.Errorf("waited %d and tried %d times, want %d waits over %d attempts",
			p.waits, p.attempts, polls, polls+1)
	}
	if len(p.activated) != 0 {
		t.Errorf("activated %v with no recorded owner; there is no PID to look for", p.activated)
	}
}

// A lock that cannot be taken at all is not evidence of another instance. Refusing to start on a
// read-only stick would turn a narrow risk into a certain outage.
func TestUnlockableDataRootStartsDegraded(t *testing.T) {
	cause := fmt.Errorf("instance lock open: %w", os.ErrPermission)
	p := &guardProbe{acquireErrs: []error{cause}}
	claim := p.guard(20).claim("data")

	if !claim.proceed {
		t.Fatalf("claim = %+v, want proceed", claim)
	}
	if !errors.Is(claim.degraded, os.ErrPermission) {
		t.Errorf("degraded = %v, want the underlying cause so the log says why", claim.degraded)
	}
	if p.attempts != 1 || p.waits+p.reports != 0 {
		t.Errorf("attempts %d, waits %d, reports %d; an unusable lock is not retried or reported",
			p.attempts, p.waits, p.reports)
	}
	claim.release()
}

func TestAlreadyRunningMessageSaysWhatToDo(t *testing.T) {
	for _, want := range []string{"already open", "overwrite", "Switch to the open window"} {
		if !strings.Contains(alreadyRunningBody, want) {
			t.Errorf("message %q does not say %q", alreadyRunningBody, want)
		}
	}
}
