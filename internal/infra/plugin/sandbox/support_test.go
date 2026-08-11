package sandbox_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/sandbox"
)

// TestSupportAlwaysExplainsWhatItDoesNotCover pins the one property that must hold on every
// platform, including the ones this build cannot confine and the ones a later build will.
//
// A sandbox status with no reason is the failure this whole pipeline exists to avoid. Unavailable
// with no reason sends the user looking through settings for a switch to turn on — "your kernel is
// too old", "this platform is not supported" and "this build does not implement it yet" are three
// different actions. Partial with no reason is worse: the user is told a boundary exists and not
// which side of it they are standing on.
func TestSupportAlwaysExplainsWhatItDoesNotCover(t *testing.T) {
	support := sandbox.Support()

	if support.Mode() == domainplugin.SandboxEnforced {
		return
	}
	if strings.TrimSpace(support.Reason) == "" {
		t.Fatalf("Support() reports %q and gives no reason; the UI and the audit log have nothing "+
			"to show but the mode", support.Mode())
	}
}
