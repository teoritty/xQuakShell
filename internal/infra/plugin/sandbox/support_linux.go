//go:build linux

package sandbox

import domainplugin "xquakshell/internal/domain/plugin"

// Support reports what this Linux build can enforce.
//
// Nothing, yet. Confinement here means a Landlock ruleset, which is self-applied and survives
// execve, so it has to be installed by a shim between fork and exec — Go offers no hook there, and
// the plugin binary cannot be asked to restrict itself. The rlimits the host already applies bound
// memory and open files, which is not a boundary on anything the plugin can read.
func Support() domainplugin.SandboxSupport {
	return domainplugin.SandboxSupport{
		Reason: "Landlock isolation is not implemented in this build; the plugin runs with your account's full access to files and the network",
	}
}
