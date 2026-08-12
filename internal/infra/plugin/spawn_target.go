package plugin

// spawnTarget is what the host actually execs for a plugin.
//
// Without OS isolation it is the plugin binary itself and no arguments, which is what every
// platform did before there was a sandbox. With it, on Linux, it is this binary re-invoked as the
// shim, and the plugin binary is named inside the arguments — the shim confines the process it is
// already in and only then becomes the plugin, because a Landlock ruleset can only be applied by
// the process it restricts.
type spawnTarget struct {
	path string
	args []string
}
