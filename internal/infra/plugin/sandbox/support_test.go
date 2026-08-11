package sandbox_test

import (
	"strings"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
)

// TestSupportAlwaysExplainsAnUnavailableSandbox pins the one property that must hold on every
// platform, including the ones this build cannot confine and the ones a later build will.
//
// An unavailable sandbox with no reason is the failure this whole status pipeline exists to avoid:
// the user reads "unavailable", finds nothing that says why, and goes looking through settings for a
// switch to turn on. The reason is the difference between "your kernel is too old", "this platform
// is not supported", and "this build does not implement it yet" — three different actions.
func TestSupportAlwaysExplainsAnUnavailableSandbox(t *testing.T) {
	support := sandbox.Support()

	if support.Available {
		if support.Reason != "" {
			t.Errorf("Reason = %q while Available is true; a reason describes why there is no sandbox", support.Reason)
		}
		return
	}

	if strings.TrimSpace(support.Reason) == "" {
		t.Fatal("Support() reports no sandbox and gives no reason; the UI and the audit log have " +
			"nothing to show but the word \"unavailable\"")
	}
}
