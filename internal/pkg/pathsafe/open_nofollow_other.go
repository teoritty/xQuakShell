//go:build !unix && !windows

package pathsafe

import "os"

func openExistingNoFollow(path string, write bool) (*os.File, error) {
	if write {
		return os.OpenFile(path, os.O_RDWR, 0)
	}
	return os.OpenFile(path, os.O_RDONLY, 0)
}

func openDirNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

func createNewNoFollow(path string, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
}
