package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

// installReadPaths is the plugin's install tree, granted entry by entry rather than as one
// directory.
//
// A user-installed plugin's data directory sits INSIDE its install tree
// (<dataRoot>/plugins/<id>/data next to <dataRoot>/plugins/<id>/bin), and a single grant on the
// install directory would carry read access to all of it — so a per-session plugin would be able to
// read every other session's instance directory, reopening precisely the isolation ADR-003
// established. Enumerating the entries and dropping the data one closes that, and costs a readdir
// per start on a directory with a handful of entries in it.
//
// Both platforms need it, and Windows needed it for a reason worth recording: an inheritable ACE
// for the container SID on the install directory propagates down the whole subtree, so session A's
// SID landed on session B's instance directory and A could read B's files. The per-identity ACL
// does not save you when the grant that reaches the file was written one level too high. The
// cross-session probe caught it.
//
// The price is that the plugin cannot list its own install directory, only read within its
// subtrees. Nothing in the plugin contract asks it to, and a plugin that does gets a denial it can
// see rather than a sibling's files it should not.
func installReadPaths(dataRoot string, plugin domainplugin.InstalledPlugin) ([]string, error) {
	// The entry path was resolved through its symlinks, so the install root has to be too or the
	// containment check between them compares two different spellings of the same directory.
	root, err := filepath.EvalSymlinks(plugin.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin %s install dir: %w", plugin.Manifest.ID, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read plugin %s install dir: %w", plugin.Manifest.ID, err)
	}
	// The same reasoning applied to the other side of the comparison, which it was missing. The
	// entries below are built from the RESOLVED root, so a data directory named from the raw data
	// root is the second spelling this function's own doc comment warns about - and on Windows an
	// 8.3 alias supplies one for free: C:\Users\RUNNER~1\... against C:\Users\runneradmin\....
	// The exclusion then matched nothing, the data directory was granted read access with the rest
	// of the install tree, and every session could read every other session's files. That is the
	// exact defect the enumeration exists to prevent, reintroduced by a spelling.
	//
	// Both forms are checked rather than one: an entry that matches either is the data directory,
	// and the safe direction here is to exclude too eagerly rather than too little.
	dataBases := []string{PluginDataDir(dataRoot, plugin.Manifest.ID)}
	if resolved, err := filepath.EvalSymlinks(dataBases[0]); err == nil {
		dataBases = append(dataBases, resolved)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if isDataDir(path, dataBases) {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("plugin %s install dir has nothing to grant", plugin.Manifest.ID)
	}
	return paths, nil
}

// isDataDir reports whether an install-tree entry is the plugin's data directory under any of the
// spellings that name it.
func isDataDir(path string, dataBases []string) bool {
	for _, dataBase := range dataBases {
		if pathsafe.UnderRoot(path, dataBase) {
			return true
		}
	}
	return false
}
