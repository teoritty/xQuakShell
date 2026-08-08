package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

// ResolveEngineEntryPath resolves engine.entry under rootDir with prefix and symlink checks.
func ResolveEngineEntryPath(rootDir, entry string) (string, error) {
	if err := domainplugin.ValidateBundleRelativePath(entry); err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve plugin root: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)

	candidate, err := pathsafe.ResolveUnderRoot(rootAbs, entry)
	if err != nil {
		return "", fmt.Errorf("%w: engine.entry escapes plugin bundle", domainplugin.ErrInvalidManifest)
	}
	resolved, err := pathsafe.SecurePathUnderRoots(candidate, []string{rootAbs})
	if err != nil {
		return "", fmt.Errorf("%w: engine.entry escapes plugin bundle", domainplugin.ErrInvalidManifest)
	}
	return resolved, nil
}

// EnsureEntryExecutable gives an already-installed plugin entry its owner-execute bit back.
//
// This repairs trees written before CopyBundle learned to keep that bit. Those installs exist on
// disk in rc.3 and earlier, and a fix that only corrected NEW installs would leave every plugin on
// every Linux machine dead until its owner happened to reinstall it - with nothing on screen to
// suggest that is the remedy, because the failure arrives as a bare
// `fork/exec …: permission denied` from the OS.
//
// It grants nothing that was not already about to happen: the caller is the spawner, one line
// before it execs this exact path, from the host's own install root, on a tree whose checksums
// were verified at install time. Adding +x to a file we are already going to run is not a new
// privilege.
//
// Skipped on Windows, which has no execute bit - Perm() reports 0666 there, so an unguarded version
// would chmod on every single plugin start and change nothing.
func EnsureEntryExecutable(entryPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(entryPath)
	if err != nil {
		return err
	}
	perm := info.Mode().Perm()
	if perm&0o100 != 0 {
		return nil
	}
	// #nosec G302 -- this ORs in the owner-execute bit and nothing else, so the result is at most
	// 0700 for the 0600 the broken installs wrote. The host cannot start a plugin without it.
	if err := os.Chmod(entryPath, perm|0o100); err != nil {
		return fmt.Errorf("make plugin entry executable: %w", err)
	}
	return nil
}
