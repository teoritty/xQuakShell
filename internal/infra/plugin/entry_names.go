package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

// entryNameCandidates returns the bundle-relative names a manifest's engine.entry may legitimately
// appear under: the declared one first, then its .exe alternate.
//
// The alternate exists because one manifest is published for every platform and can only name the
// binary once — a Windows-shaped "xqs-vnc.exe" is the same entry as a POSIX "xqs-vnc". Both the
// installed-plugin loader and the release-archive reader have to accept that pair, and they have
// to accept exactly the same pair, which is why the rule lives here rather than in either of them.
func entryNameCandidates(entry string) []string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}
	if strings.HasSuffix(strings.ToLower(entry), ".exe") {
		return []string{entry, entry[:len(entry)-len(".exe")]}
	}
	return []string{entry, entry + ".exe"}
}

// resolveEntryAlternate returns the path of the .exe alternate of entry when it exists in dir,
// or "" when it does not.
func resolveEntryAlternate(dir, entry string) string {
	for _, name := range entryNameCandidates(entry)[1:] {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
