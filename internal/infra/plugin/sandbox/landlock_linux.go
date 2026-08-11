//go:build linux

package sandbox

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Landlock ABI versions, each named for what it added and the kernel that shipped it. A ruleset
// that handles a right the running kernel does not know is rejected outright with EINVAL, and a
// rule granting one is rejected the same way, so every mask below is a function of the probed
// version rather than a constant.
//
// A kernel newer than the last entry is used as if it were that entry: rights added after
// abiIoctlDev are neither handled nor granted, which leaves them unrestricted. That is the safe
// direction for compatibility and the unsafe one for confinement, and it is why a new ABI is worth
// reading the changelog for rather than ignoring.
const (
	abiFilesystem = 1 // 5.13: the filesystem rights this whole design rests on
	abiRefer      = 2 // 5.19: linking or renaming between two granted directories
	abiTruncate   = 3 // 6.2:  truncating a file, which opening one with O_TRUNC needs
	abiNetwork    = 4 // 6.7:  binding and connecting TCP
	abiIoctlDev   = 5 // 6.10: ioctl on a character or block device
)

// landlockABI reports the Landlock ABI version the running kernel implements.
//
// The probe is landlock_create_ruleset itself, called with the version flag and no attributes: the
// kernel answers with a number instead of creating anything. ENOSYS means the kernel was built
// without Landlock, EOPNOTSUPP means it was built with it and booted without it — two different
// things to tell a user, and both reach them through SandboxSupport.Reason.
func landlockABI() (int, error) {
	version, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, errno
	}
	return int(version), nil
}

// handledAccessFS is every filesystem right the ruleset takes responsibility for. A right that is
// handled is denied unless some rule grants it; a right that is not handled is not governed at all,
// which is why this list is as long as the ABI allows rather than as long as the grants need.
func handledAccessFS(abi int) uint64 {
	access := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= abiRefer {
		access |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= abiTruncate {
		access |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= abiIoctlDev {
		access |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return access
}

// handledAccessNet is the network rights the ruleset takes responsibility for, which below kernel
// 6.7 is none of them.
//
// Even at abiNetwork this is not the whole network: Landlock governs TCP bind and connect and says
// nothing about UDP, raw or packet sockets. Denying TCP is worth doing — no plugin has a legitimate
// use for any socket (the host dials on its behalf) — but it is why a Landlock-confined process
// reports SandboxEnforcedPartial rather than SandboxEnforced however new the kernel is.
func handledAccessNet(abi int) uint64 {
	if abi < abiNetwork {
		return 0
	}
	return unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
}

// accessReadOnly is what a process may do with a path it needs only to read.
func accessReadOnly(int) uint64 {
	return unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
}

// accessReadExecute additionally lets the process run what it finds there. The plugin's own install
// tree needs it to be exec'd at all, and the system library paths need it because loading a shared
// object is an execute of that file.
func accessReadExecute(abi int) uint64 {
	return accessReadOnly(abi) | unix.LANDLOCK_ACCESS_FS_EXECUTE
}

// accessReadWrite is the grant for the plugin's own instance directory: the ordinary file
// operations and nothing else.
//
// EXECUTE is deliberately absent, so a plugin cannot write a binary into its data directory and
// run it. That is not much of a barrier — the plugin is already running code of its own choosing —
// but it costs nothing and it removes the shortest path from "wrote a file" to "ran a file" for
// anything that later compromises the plugin. MAKE_CHAR, MAKE_BLOCK, MAKE_SOCK and MAKE_FIFO are
// absent for the same reason: nothing in a plugin needs them, so granting them only widens what a
// compromised one can build.
func accessReadWrite(abi int) uint64 {
	access := uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM)
	if abi >= abiRefer {
		access |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= abiTruncate {
		access |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return access
}

// ruleset is a Landlock ruleset under construction and the descriptor that owns it.
type ruleset struct {
	fd  int
	abi int
}

// newRuleset opens a ruleset that denies everything the ABI lets it govern. Rules added to it only
// ever punch holes; there is no deny rule and no way to narrow it again afterwards.
func newRuleset(abi int) (*ruleset, error) {
	attr := unix.LandlockRulesetAttr{
		Access_fs:  handledAccessFS(abi),
		Access_net: handledAccessNet(abi),
	}
	// The attribute struct grows with the ABI, and this one is whatever the vendored x/sys knows.
	// Handing an older kernel a longer struct is allowed precisely because the fields it does not
	// know are zero here: it checks the tail is zeroed and copies the prefix it understands.
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return nil, fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	return &ruleset{fd: int(fd), abi: abi}, nil
}

// allow grants access beneath one path. A path that does not exist is an error: every path the
// caller passes here is one it just built or just resolved, so a missing one means the layout
// changed under us and the confinement would not be the one that was described.
func (r *ruleset) allow(path string, access uint64) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s for landlock rule: %w", path, err)
	}
	defer func() { _ = unix.Close(fd) }()

	allowed, err := applicableAccess(fd, access&handledAccessFS(r.abi))
	if err != nil {
		return fmt.Errorf("stat %s for landlock rule: %w", path, err)
	}
	attr := unix.LandlockPathBeneathAttr{
		// The mask above keeps a grant from naming a right the ruleset does not handle, which the
		// kernel refuses outright; both sides are derived from the same ABI, so it never actually
		// removes anything.
		Allowed_access: allowed,
		// #nosec G115 -- the kernel's own type for a descriptor here is __s32, and unix.Open has
		// just returned this one: a Linux fd is a small non-negative int bounded by RLIMIT_NOFILE,
		// orders of magnitude below where an int32 stops holding it.
		Parent_fd: int32(fd),
	}
	if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(r.fd),
		unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&attr)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("landlock_add_rule for %s: %w", path, errno)
	}
	return nil
}

// fileAccessRights are the rights that mean something for a path that is not a directory. The rest
// describe operations on directory entries, and naming one of them in a rule whose target is a
// regular file or a device node is not a harmless over-grant — the kernel rejects the whole rule
// with EINVAL.
const fileAccessRights = uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
	unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
	unix.LANDLOCK_ACCESS_FS_TRUNCATE |
	unix.LANDLOCK_ACCESS_FS_IOCTL_DEV)

// applicableAccess narrows a grant to what the target can actually be granted, so that a caller can
// name one access set for a mixed list of paths. The plugin's install tree is exactly that: the
// entries are granted one by one, and some of them are files.
func applicableAccess(fd int, access uint64) (uint64, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return 0, err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		return access, nil
	}
	return access & fileAccessRights, nil
}

// allowIfPresent is allow for a path the machine may simply not have — /lib64 on a 32-bit install,
// /libx32 nearly everywhere. Absence is not a failure; anything else is.
func (r *ruleset) allowIfPresent(path string, access uint64) error {
	err := r.allow(path, access)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}

// restrictSelf puts the calling process inside the ruleset, for good.
//
// PR_SET_NO_NEW_PRIVS is not optional bookkeeping: landlock_restrict_self refuses with EPERM
// without it, because a domain that a setuid binary could exec its way out of would not be a
// domain. It is also what makes the confinement survive the execve into the plugin.
func (r *ruleset) restrictSelf() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(r.fd), 0, 0); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

func (r *ruleset) close() {
	_ = unix.Close(r.fd)
}
