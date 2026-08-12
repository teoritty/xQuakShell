package plugin

import (
	"os"
	"testing"

	"xquakshell/internal/infra/plugin/sandbox"
)

// TestMain answers the sandbox shim flag before the suite runs, for the same reason the suite in
// test/unit/plugin does: these tests spawn real processes through spawnPluginProcess, which on a
// platform that can confine one re-invokes the host binary as the shim — and here the host binary
// is this test binary.
func TestMain(m *testing.M) {
	if sandbox.IsShimMode(os.Args) {
		sandbox.RunShim(os.Args)
	}
	os.Exit(m.Run())
}
