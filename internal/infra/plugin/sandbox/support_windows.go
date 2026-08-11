//go:build windows

// Package sandbox answers one question per platform: can this build confine a plugin process to
// its own directories, and if not, why not.
//
// It deliberately holds no policy. What to do about an answer — start anyway, refuse, or let the
// user override — is a decision for the usecase layer, which is where the setting that governs it
// lives. This package reports facts about the operating system.
package sandbox

import domainplugin "xquakshell/internal/domain/plugin"

// Support reports what this Windows build can enforce.
//
// Nothing, yet. Confinement here means an AppContainer, which cannot be reached through os/exec:
// syscall.SysProcAttr exposes Token but no attribute list, so PROC_THREAD_ATTRIBUTE_SECURITY_
// CAPABILITIES cannot be passed and the child has to be created by hand. The job object the host
// already applies bounds memory, process count and the shared USER surface — none of which is a
// boundary on the filesystem or the network.
func Support() domainplugin.SandboxSupport {
	return domainplugin.SandboxSupport{
		Reason: "AppContainer isolation is not implemented in this build; the plugin runs with your account's full access to files and the network",
	}
}
