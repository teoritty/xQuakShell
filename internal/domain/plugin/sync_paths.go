package plugin

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Limits on what one plugin may sync. A file or a plugin over them is left out and reported rather
// than truncated: a partial file is worse than none to the plugin that reads it.
const (
	MaxSyncFileBytes   = 16 << 20
	MaxSyncPluginBytes = 64 << 20
	MaxSyncPluginFiles = 4096
	MaxSyncPatterns    = 32
	maxSyncPathLen     = 256
)

// ErrUnsafeSyncPath is a synced file path that could reach outside the plugin's data directory, or
// into the part of it that is never synced.
var ErrUnsafeSyncPath = errors.New("plugin: unsafe sync path")

// SyncSpec declares which files under the plugin's data directory follow the user to their other
// devices. Paths are slash-separated and relative to ${pluginData}; '*' matches within one segment
// and a '**' segment matches any number of segments. Optional, and omitempty on Manifest.Sync is
// load-bearing for the reason BundleFormat gives.
type SyncSpec struct {
	Paths []string `json:"paths,omitempty"`
}

// Validate applies the path policy to every declared pattern. A per-session plugin cannot sync:
// its data root holds only per-session instance directories, which never leave the device.
func (s *SyncSpec) Validate(isolation IsolationMode) error {
	if s == nil {
		return nil
	}
	if isolation == IsolationPerSession && len(s.Paths) > 0 {
		return fmt.Errorf("%w: sync is not available with per-session isolation", ErrInvalidManifest)
	}
	if len(s.Paths) > MaxSyncPatterns {
		return fmt.Errorf("%w: more than %d sync paths", ErrInvalidManifest, MaxSyncPatterns)
	}
	for _, p := range s.Paths {
		if err := checkSyncSegments(p, true); err != nil {
			return fmt.Errorf("%w: sync path %q: %w", ErrInvalidManifest, p, err)
		}
	}
	return nil
}

// Matches reports whether a file, relative to the plugin data directory, is one the plugin declared
// for sync. A path the policy refuses never matches, whatever the patterns say.
func (s *SyncSpec) Matches(rel string) bool {
	if s == nil || ValidSyncFile(rel) != nil {
		return false
	}
	file := strings.Split(rel, "/")
	for _, p := range s.Paths {
		if matchSegments(strings.Split(p, "/"), file) {
			return true
		}
	}
	return false
}

// ValidSyncFile checks one concrete file path, as it arrives from another device or from a scan.
// It is the check that keeps a hostile device's record from naming ../vault.age.
func ValidSyncFile(rel string) error {
	if err := checkSyncSegments(rel, false); err != nil {
		return fmt.Errorf("%w: %q: %w", ErrUnsafeSyncPath, rel, err)
	}
	return nil
}

// checkSyncSegments is the policy shared by patterns and files. Everything is refused that a
// filesystem on any supported OS could read as something other than a plain name under the root:
// separators other than '/', a colon (a drive or an NTFS alternate data stream), "." and "..",
// names Windows reserves for devices or silently trims, and control characters.
func checkSyncSegments(p string, pattern bool) error {
	if p == "" || len(p) > maxSyncPathLen {
		return errors.New("empty or too long")
	}
	if strings.ContainsAny(p, `\:`) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "$") {
		return errors.New("not a relative slash-separated path")
	}
	for seg := range strings.SplitSeq(p, "/") {
		if err := checkSyncSegment(seg, pattern); err != nil {
			return err
		}
	}
	return nil
}

func checkSyncSegment(seg string, pattern bool) error {
	switch {
	case seg == "" || seg == "." || seg == "..":
		return errors.New("empty, '.' or '..' segment")
	case strings.EqualFold(seg, "tmp"):
		return errors.New("tmp is never synced")
	case strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " "):
		return errors.New("a name ending in '.' or ' '")
	case isWindowsDeviceName(seg):
		return errors.New("a reserved device name")
	case strings.IndexFunc(seg, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0:
		return errors.New("a control character")
	}
	if !pattern {
		return nil
	}
	if strings.ContainsAny(seg, "?[]{}") {
		return errors.New("only '*' and '**' are supported")
	}
	if strings.Contains(seg, "**") && seg != "**" {
		return errors.New("'**' must be a whole segment")
	}
	return nil
}

// isWindowsDeviceName reports CON, PRN, AUX, NUL, COM1-9 and LPT1-9, with or without an extension:
// on Windows "nul.json" opens the null device, so a file by that name syncs into nothing.
func isWindowsDeviceName(seg string) bool {
	base, _, _ := strings.Cut(strings.ToLower(seg), ".")
	switch base {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9'
}

// matchSegments matches a validated pattern against a validated file, segment by segment.
func matchSegments(pat, file []string) bool {
	if len(pat) == 0 {
		return len(file) == 0
	}
	if pat[0] == "**" {
		// A trailing "**" names the files under a directory, never the directory's own name.
		start := 0
		if len(pat) == 1 {
			start = 1
		}
		for i := start; i <= len(file); i++ {
			if matchSegments(pat[1:], file[i:]) {
				return true
			}
		}
		return false
	}
	if len(file) == 0 {
		return false
	}
	// The pattern was validated to carry no metacharacter but '*', so path.Match cannot fail here
	// and reads the file segment literally.
	ok, err := path.Match(pat[0], file[0])
	return err == nil && ok && matchSegments(pat[1:], file[1:])
}
