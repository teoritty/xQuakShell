//go:build !linux

package sandbox

import (
	"fmt"
	"os"
)

// RunShim exists on every platform so that main can dispatch on IsShimMode without a build tag of
// its own, mirroring the log viewer's single early check.
//
// Nothing produces this argv here — Support reports no confinement, so no spawn ever asks for the
// shim — and the flag arriving anyway means something is wrong rather than that a plugin should be
// launched unconfined. Exiting is the same refusal the Linux shim makes for the same reason.
func RunShim(_ []string) {
	fmt.Fprintln(os.Stderr, "plugin sandbox: this build has no sandbox shim")
	os.Exit(1)
}
