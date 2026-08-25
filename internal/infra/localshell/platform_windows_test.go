//go:build windows

package localshell

import (
	"strings"
	"testing"
)

// TestPowerShellRedirectKeepsItsSpaces guards a whitespace-sensitive corner of PowerShell's
// parser that produced a visible error in every terminal the application opened.
//
// `chcp 65001>$null` does not parse as a command plus a redirect. PowerShell hands chcp the
// argument `65001>`, which answers "Invalid parameter format" - printed into the terminal the
// user just asked for, before their first prompt. Nothing about the string looks wrong, so the
// spaces are exactly the kind of thing a later tidy-up removes.
func TestPowerShellRedirectKeepsItsSpaces(t *testing.T) {
	args := strings.Join(startupArgs("powershell"), " ")

	if !strings.Contains(args, "chcp 65001 > $null") {
		t.Errorf("powershell startup is %q; the redirect needs whitespace on both sides or chcp "+
			"receives \"65001>\" as its parameter and reports an error into the new terminal", args)
	}
}

// TestPowerShell7NeedsNoEncodingFixup records why one shell gets no arguments at all: pwsh
// already defaults to UTF-8, so adding a chcp for symmetry would be a startup banner for nothing.
func TestPowerShell7NeedsNoEncodingFixup(t *testing.T) {
	if got := startupArgs("pwsh"); len(got) != 0 {
		t.Errorf("startupArgs(pwsh) = %v, want none", got)
	}
}

// TestCmdRedirectsIntoNul keeps the "Active code page: 65001" line out of a fresh cmd.
func TestCmdRedirectsIntoNul(t *testing.T) {
	args := strings.Join(startupArgs("cmd"), " ")
	if !strings.Contains(args, "chcp 65001") || !strings.Contains(args, "nul") {
		t.Errorf("cmd startup is %q; want a code page switch whose output is discarded", args)
	}
}
