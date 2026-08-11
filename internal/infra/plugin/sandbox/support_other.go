//go:build !windows && !linux

package sandbox

import domainplugin "xquakshell/internal/domain/plugin"

// Support reports what this platform can enforce.
//
// Nothing, and on macOS that is the settled position rather than a gap waiting to be filled: the
// isolation work covers Windows and Linux, so this answer is expected to outlive the other two.
// Saying so here, in the string a user reads, is the point — an "unavailable" that looks temporary
// invites someone to wait for a release that is not coming.
func Support() domainplugin.SandboxSupport {
	return domainplugin.SandboxSupport{
		Reason: "OS-level plugin isolation is not available on this platform; the plugin runs with your account's full access to files and the network",
	}
}
