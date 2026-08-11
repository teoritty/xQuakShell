package plugin_test

import (
	"os"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
)

// TestMain answers the sandbox shim flag before the suite runs.
//
// On a platform that can confine a plugin, the spawner starts the host binary again with a shim
// argv and lets that process narrow itself before it becomes the plugin. In this suite the host
// binary is this test binary, so it has to answer that argv exactly as main does — otherwise every
// plugin start re-runs the whole suite as a child, the handshake never arrives, and the failure
// reads as "plugin initialize: EOF" with nothing pointing at the cause.
//
// That coupling is the point rather than a workaround: these tests start real fixture binaries, so
// with this in place they exercise the real shim, and a plugin that cannot live inside the ruleset
// fails here rather than on a user's machine.
func TestMain(m *testing.M) {
	if sandbox.IsShimMode(os.Args) {
		sandbox.RunShim(os.Args)
	}
	os.Exit(m.Run())
}
