//go:build unix

package pathsafe

import (
	"os"

	"golang.org/x/sys/unix"
)

func openExistingNoFollow(path string, write bool) (*os.File, error) {
	flags := unix.O_NOFOLLOW | unix.O_CLOEXEC
	if write {
		flags |= unix.O_RDWR
	} else {
		flags |= unix.O_RDONLY
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func openDirNoFollow(path string) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_DIRECTORY
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func createNewNoFollow(path string, perm os.FileMode) (*os.File, error) {
	flags := unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC
	fd, err := unix.Open(path, flags, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
