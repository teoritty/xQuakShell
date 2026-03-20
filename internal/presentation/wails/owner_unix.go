//go:build !windows

package wails

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func getLocalFileOwner(info os.FileInfo, _ string) string {
	if info == nil {
		return ""
	}
	sys := info.Sys()
	if sys == nil {
		return ""
	}
	stat, ok := sys.(*syscall.Stat_t)
	if !ok {
		return ""
	}
	owner, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
	if err != nil {
		return strconv.FormatUint(uint64(stat.Uid), 10)
	}
	return owner.Username
}
