package portable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// instanceLockName is the file whose operating-system lock marks a data root as in use.
const instanceLockName = "instance.lock"

// ErrInstanceRunning means another live process holds the data root's instance lock.
//
// It is kept apart from every other acquisition failure because it is the only one that says
// something about the data: the others - a read-only stick, a filesystem without locks - say only
// that this process cannot take part in the protocol, and the caller treats the two differently.
var ErrInstanceRunning = errors.New("another process is using this data root")

// InstanceLock is an exclusive, operating-system-level claim on one data root.
//
// Two processes working on one data root each hold their own decrypted copy of the vault and
// write it back whole, so whichever saves last silently discards everything the other one changed:
// a key generated in one window is gone the next time the other one saves a connection. The atomic
// rename in the vault writer does not help - it prevents a torn file, not a lost update - and both
// processes would also share vault.age.tmp as their scratch name.
//
// The lock is a byte-range lock (LockFileEx) on Windows and flock elsewhere, never a PID file. The
// kernel drops either when the process ends, however it ends, so a crash, a kill or a USB stick
// pulled mid-session cannot leave a stale claim behind that a user would have to find and delete.
// The PID written into the file is for the process that loses the race, to find the winner's
// window; it is never consulted to decide whether the lock is held.
type InstanceLock struct {
	once sync.Once
	file *os.File
	err  error
}

// InstanceLockPath is where the lock file for a data root lives.
func InstanceLockPath(dataRoot string) string {
	return filepath.Join(dataRoot, instanceLockName)
}

// AcquireInstanceLock claims dataRoot for this process without waiting.
//
// It returns an error wrapping ErrInstanceRunning when another process holds the claim, and any
// other error when the claim cannot be attempted at all.
//
// The file handle does not reach child processes: Go opens files close-on-exec on Unix and
// non-inheritable on Windows, so a plugin or the log viewer outliving this process cannot keep the
// data root claimed on its behalf.
func AcquireInstanceLock(dataRoot string) (*InstanceLock, error) {
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return nil, fmt.Errorf("instance lock mkdir %s: %w", dataRoot, err)
	}
	path := InstanceLockPath(dataRoot)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("instance lock open %s: %w", path, err)
	}
	if err := lockFile(file); err != nil {
		closeErr := file.Close()
		return nil, errors.Join(fmt.Errorf("instance lock %s: %w", path, err), closeErr)
	}
	// Only once the lock is ours: the loser of a race must not truncate the winner's PID.
	if err := recordOwner(file); err != nil {
		lock := &InstanceLock{file: file}
		return nil, errors.Join(fmt.Errorf("instance lock record owner %s: %w", path, err), lock.Release())
	}
	return &InstanceLock{file: file}, nil
}

// Release gives the data root up. Safe to call more than once; later calls return the first
// call's result.
//
// The file itself is left in place. Deleting it would reopen the race the lock exists to close:
// on Unix a process that opened the old file just before the unlink goes on to lock an inode no
// other process can reach again, and two instances would each believe they hold the data root.
func (l *InstanceLock) Release() error {
	l.once.Do(func() {
		unlockErr := unlockFile(l.file)
		l.err = errors.Join(unlockErr, l.file.Close())
	})
	return l.err
}

// InstanceLockOwner returns the PID the current holder of dataRoot's lock recorded, if any.
//
// Advisory only: the holder may have exited since, and on a stale file the PID may by now belong
// to an unrelated process. Callers use it to find a window, never to decide whether to start.
func InstanceLockOwner(dataRoot string) (int, bool) {
	raw, err := os.ReadFile(InstanceLockPath(dataRoot))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func recordOwner(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	_, err := file.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	return err
}
