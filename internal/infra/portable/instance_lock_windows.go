//go:build windows

package portable

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockOffsetHigh places the locked byte far past anything the file will ever hold.
//
// Windows byte-range locks are mandatory: while held, no other process can read the bytes they
// cover. Locking byte 0 would hide the owner's PID from the one process that needs to read it - the
// second instance looking for the first one's window. Windows accepts a lock beyond end of file.
const lockOffsetHigh = 0x7FFFFFFF

func lockOverlapped() *windows.Overlapped {
	return &windows.Overlapped{OffsetHigh: lockOffsetHigh}
}

func lockFile(file *os.File) error {
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, lockOverlapped(),
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrInstanceRunning
	}
	return err
}

func unlockFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, lockOverlapped())
}
