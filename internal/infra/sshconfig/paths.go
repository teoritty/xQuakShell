package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath returns the user's conventional OpenSSH client config path when
// that file exists, or an empty string when it does not.
//
// A missing config is an ordinary state, not an error: plenty of users have
// never written one, and the import dialog should open with an empty field
// rather than an error banner.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	candidate := filepath.Join(home, ".ssh", "config")
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return "", nil
	}
	return candidate, nil
}

// expandTilde resolves a leading "~" or "~/" against homeDir.
//
// "~user/..." is intentionally left untouched: resolving another account's
// home directory would mean reading outside the current user's own files, and
// OpenSSH's own tilde handling for foreign users is not something the importer
// needs in order to read the user's own configuration.
func expandTilde(path, homeDir string) string {
	if homeDir == "" || path == "" || path[0] != '~' {
		return path
	}
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// userHomeDir returns the current user's home directory, or "" when it cannot
// be determined. Callers degrade gracefully rather than failing the parse.
func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// currentUserName returns the local account name used to expand the %u token.
func currentUserName() string {
	for _, key := range []string{"USER", "USERNAME", "LOGNAME"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
