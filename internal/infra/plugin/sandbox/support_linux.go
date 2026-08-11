//go:build linux

package sandbox

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"

	domainplugin "xquakshell/internal/domain/plugin"
)

// Support reports what this Linux build can enforce, by asking the kernel for its Landlock ABI.
//
// The answer is never whole confinement, however new the kernel is. Landlock's network rules govern
// TCP bind and connect and nothing else, so a plugin behind them can still open a UDP or raw socket
// — which is enough to exfiltrate anything it can read. The filesystem side is complete, and the
// filesystem is what holds the vault, the SSH keys and the other plugins' data, so this is a real
// boundary reported as the partial one it is.
func Support() domainplugin.SandboxSupport {
	abi, err := landlockABI()
	if err != nil {
		return domainplugin.SandboxSupport{Reason: unavailableReason(err)}
	}
	if abi < abiFilesystem {
		return domainplugin.SandboxSupport{
			Reason: fmt.Sprintf("this kernel reports Landlock ABI %d, which cannot confine a "+
				"plugin's files; the plugin runs with your account's full access", abi),
		}
	}
	return domainplugin.SandboxSupport{
		Available: true,
		Reason:    partialReason(abi),
	}
}

// unavailableReason turns the probe's errno into something a user can act on. The two it can
// realistically return mean different things and have different fixes, and "unavailable" without
// that distinction sends someone looking through this application's settings for a kernel option.
func unavailableReason(err error) string {
	switch {
	case errors.Is(err, unix.ENOSYS):
		return "this kernel was built without Landlock, so the plugin runs with your account's " +
			"full access to files and the network"
	case errors.Is(err, unix.EOPNOTSUPP):
		return "this kernel has Landlock but was booted without it enabled (add landlock to the " +
			"lsm= boot parameter); the plugin runs with your account's full access"
	default:
		return fmt.Sprintf("Landlock could not be probed (%v), so the plugin runs with your "+
			"account's full access to files and the network", err)
	}
}

func partialReason(abi int) string {
	if abi < abiNetwork {
		return fmt.Sprintf("the plugin's files are confined to its own directory; this kernel's "+
			"Landlock (ABI %d, network rules need ABI %d) cannot restrict its network access",
			abi, abiNetwork)
	}
	return "the plugin's files are confined to its own directory and it cannot open a TCP " +
		"connection; Landlock does not cover UDP or raw sockets"
}
