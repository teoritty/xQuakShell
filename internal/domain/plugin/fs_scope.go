package plugin

import (
	"slices"
	"strings"
)

// PluginDataPlaceholder is the manifest token that expands to the plugin's own data directory.
// A pattern built from it is scoped by construction and is the shape a well-behaved manifest uses.
const PluginDataPlaceholder = "${pluginData}"

// IsBroadFilesystemPattern reports whether an fs pattern grants so much that calling it a sandbox
// path is misleading.
//
// The install consent screen used to summarise any fs capability as "Read files in declared
// sandbox paths" without naming them, and nothing validated what was declared. A manifest could
// ask for "/" or "C:\" and the user would be shown a sentence that sounds bounded. The word
// "sandbox" was doing the reassuring while the paths did the opposite.
//
// What counts as broad:
//
//   - a filesystem root, or a bare volume root on Windows ("/", "C:\", "\\");
//   - a pattern that walks out of wherever it starts (".." at the front or in the middle), since
//     what it finally covers depends on a base directory the manifest does not control;
//   - "~" or "~/" on its own - the whole home directory, which holds ~/.ssh among everything else.
//
// A pattern anchored on ${pluginData} is never broad: it cannot escape the plugin's own directory
// once resolveRoots has expanded and cleaned it.
func IsBroadFilesystemPattern(pattern string) bool {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, PluginDataPlaceholder) {
		return false
	}

	// Separators are normalised by hand rather than with filepath.ToSlash, which is a no-op on
	// Linux: a manifest is inspected wherever the install happens, and a Windows-authored "C:\\"
	// has to be recognised on a Linux CI runner as readily as on the machine it targets.
	unix := strings.ReplaceAll(trimmed, "\\", "/")
	if unix == "/" || unix == "//" || unix == "~" || unix == "~/" {
		return true
	}
	if hasTraversal(unix) {
		return true
	}
	// A Windows volume with nothing under it: "C:", "C:\", "C:/". filepath.Clean on the platform
	// that understands them collapses each to the same shape, but this has to hold when the
	// manifest is inspected on a different OS than the one it will install on.
	if len(unix) >= 2 && unix[1] == ':' && strings.Trim(unix[2:], "/") == "" {
		return true
	}
	return false
}

// hasTraversal reports whether a slash-separated pattern contains a ".." segment.
func hasTraversal(unix string) bool {
	return slices.Contains(strings.Split(unix, "/"), "..")
}

// BroadFilesystemPatterns returns every declared fs pattern that IsBroadFilesystemPattern accepts,
// read and write together, in declaration order and without duplicates.
func (m *Manifest) BroadFilesystemPatterns() []string {
	if m == nil || m.Capabilities.FS == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var broad []string
	for _, pattern := range append(append([]string{}, m.Capabilities.FS.Read...), m.Capabilities.FS.Write...) {
		if !IsBroadFilesystemPattern(pattern) {
			continue
		}
		if _, dup := seen[pattern]; dup {
			continue
		}
		seen[pattern] = struct{}{}
		broad = append(broad, pattern)
	}
	return broad
}

// RequiresBroadFilesystemAccess reports whether the manifest asks for filesystem access wide enough
// to deserve its own warning at install time, the way arbitrary outbound network access does.
func (m *Manifest) RequiresBroadFilesystemAccess() bool {
	return len(m.BroadFilesystemPatterns()) > 0
}

// filesystemPermissionLines renders the fs capability for the install consent screen.
//
// It lives here rather than in PermissionSummary because that file is at its size budget, and
// because these three lines are the whole reason this file exists: naming what is granted, and
// saying plainly when what is granted is everything.
func (m *Manifest) filesystemPermissionLines() []string {
	if m == nil || m.Capabilities.FS == nil {
		return nil
	}
	var lines []string
	// The paths are named, the way the outbound network patterns always have been. This used to
	// read "Read files in declared sandbox paths" with no list, so a manifest asking for "/" was
	// presented to the user in the language of a sandbox.
	if len(m.Capabilities.FS.Read) > 0 {
		lines = append(lines, "Read files: "+strings.Join(m.Capabilities.FS.Read, ", "))
	}
	if len(m.Capabilities.FS.Write) > 0 {
		lines = append(lines, "Write files: "+strings.Join(m.Capabilities.FS.Write, ", "))
	}
	if broad := m.BroadFilesystemPatterns(); len(broad) > 0 {
		lines = append(lines, "This covers your whole filesystem or home directory: "+strings.Join(broad, ", "))
	}
	return lines
}
