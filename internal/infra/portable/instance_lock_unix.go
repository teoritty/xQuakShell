//go:build unix

package portable

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// flock rather than fcntl record locks. fcntl locks belong to the process and are dropped when it
// closes any descriptor for the file - including one opened by a library for an unrelated reason -
// while flock belongs to the open file description this package alone holds. Linux emulates flock
// over NFS with fcntl locks, so a data root on a network share is still covered.
func lockFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EWOULDBLOCK):
			return ErrInstanceRunning
		default:
			return err
		}
	}
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
