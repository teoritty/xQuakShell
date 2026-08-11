//go:build linux

package plugin

import (
	"fmt"

	"xquakshell/internal/infra/plugin/sandbox"
)

func applyLinuxResourceLimits(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid plugin pid")
	}
	return sandbox.ApplyProcessLimits(pid)
}
