//go:build linux

package sandbox

// systemReadExecutePaths are the directories a dynamically linked binary must reach to start at
// all: the loader, the C library and everything else it pulls in.
//
// A statically linked Go plugin needs none of them, and granting them costs that plugin nothing —
// they hold no user data. Narrowing this to the exact objects a given binary loads is not possible
// from here: the set depends on the binary, its transitive dependencies and the distribution's
// layout, and getting it wrong means the plugin dies at exec with a loader error.
var systemReadExecutePaths = []string{
	"/usr",
	"/lib",
	"/lib64",
	"/lib32",
	"/libx32",
	"/bin",
	"/sbin",
}

// systemReadOnlyPaths hold configuration the loader and the C library read on the way up:
// /etc/ld.so.cache to find libraries, /etc/localtime for the timezone, /etc/nsswitch.conf and
// friends for name resolution.
//
// The whole of /etc is granted rather than that list, because the list is distribution-specific
// and a miss is a start failure. It is a real widening and a small one: /etc holds system
// configuration that any account on the machine can already read, and none of the things this
// sandbox exists to protect — the vault, the SSH keys, the other plugins' data — are in it.
var systemReadOnlyPaths = []string{
	"/etc",
}

// systemDeviceReadWritePaths and systemDeviceReadOnlyPaths are the character devices a runtime
// expects to exist. /dev itself is deliberately not granted: it is where the raw disks are.
var (
	systemDeviceReadWritePaths = []string{"/dev/null", "/dev/zero"}
	systemDeviceReadOnlyPaths  = []string{"/dev/urandom", "/dev/random"}
)

// applyLandlock confines the calling process to what args describe plus the system paths above, and
// nothing else. It does not return on success in any useful sense — the process it was called in is
// permanently narrower afterwards — and every error it returns leaves the process unconfined, which
// is why its only caller exits rather than continuing.
//
// "Process" above is the word that hid a bug for as long as it stood: Landlock confines a THREAD.
// applyLandlock confines the calling goroutine's thread, and returns which thread that was, because
// Go may move the goroutine off it at any scheduling point. The sentence above is true only for as
// long as the caller is still on the returned thread, and it is the caller's job to check that
// before it relies on it - see RunShim.
func applyLandlock(abi int, args ShimArgs) (int, error) {
	rs, err := newRuleset(abi)
	if err != nil {
		return 0, err
	}
	defer rs.close()

	// Order is not significant to the kernel; these are grouped by who they are for.
	for _, path := range args.AllowRX {
		if err := rs.allow(path, accessReadExecute(abi)); err != nil {
			return 0, err
		}
	}
	for _, path := range args.AllowRW {
		if err := rs.allow(path, accessReadWrite(abi)); err != nil {
			return 0, err
		}
	}
	if err := allowSystemPaths(rs, abi); err != nil {
		return 0, err
	}
	return rs.restrictSelf()
}

func allowSystemPaths(rs *ruleset, abi int) error {
	groups := []struct {
		paths  []string
		access uint64
	}{
		{systemReadExecutePaths, accessReadExecute(abi)},
		{systemReadOnlyPaths, accessReadOnly(abi)},
		{systemDeviceReadOnlyPaths, accessReadOnly(abi)},
		// A rule whose target is a device node keeps only the rights a non-directory can carry, so
		// the directory grant is the right thing to name here: it narrows itself.
		{systemDeviceReadWritePaths, accessReadWrite(abi)},
	}
	for _, group := range groups {
		for _, path := range group.paths {
			if err := rs.allowIfPresent(path, group.access); err != nil {
				return err
			}
		}
	}
	return nil
}
