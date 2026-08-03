package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xquakshell/internal/domain"
)

// identityTokens carries the values ssh_config's percent tokens expand to for
// one particular host.
type identityTokens struct {
	hostName string // %h — the remote host name
	alias    string // %n — the original host name as given on the command line
	user     string // %r — the remote user name
}

// resolveIdentityFiles expands each IdentityFile entry and keeps the ones that
// point at a readable regular file.
//
// Non-existent paths are dropped rather than carried forward: an IdentityFile
// line commonly names a key that lives on another machine, and importing a
// dangling reference would produce a connection that fails at authentication
// time with no explanation. The user is told through a notice instead.
func (r *resolver) resolveIdentityFiles(raw []string, tokens identityTokens, subject string) []string {
	var (
		out  []string
		seen = map[string]bool{}
	)
	for _, entry := range raw {
		path := r.expandIdentityPath(entry, tokens)
		if path == "" {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		if !isReadableRegularFile(path) {
			r.notices.add(domain.SSHConfigNoticeIdentityFileMissing, subject)
			continue
		}
		out = append(out, path)
	}
	return out
}

// expandIdentityPath resolves tildes and percent tokens to an absolute path.
func (r *resolver) expandIdentityPath(entry string, tokens identityTokens) string {
	entry = strings.TrimSpace(entry)
	if entry == "" || strings.EqualFold(entry, "none") {
		return ""
	}
	expanded := expandPercentTokens(entry, r.homeDir, tokens)
	expanded = expandTilde(expanded, r.homeDir)
	if !filepath.IsAbs(expanded) && r.homeDir != "" {
		// OpenSSH resolves a relative IdentityFile against the user's home
		// directory, not the working directory.
		expanded = filepath.Join(r.homeDir, expanded)
	}
	return filepath.Clean(expanded)
}

// expandPercentTokens implements the ssh_config(5) token subset that can
// appear in an IdentityFile path. Unknown tokens are left untouched so that a
// path is never silently mangled into a different file.
func expandPercentTokens(value, homeDir string, tokens identityTokens) string {
	if !strings.Contains(value, "%") {
		return value
	}
	replacer := strings.NewReplacer(
		"%%", "%",
		"%d", homeDir,
		"%u", currentUserName(),
		"%h", tokens.hostName,
		"%n", tokens.alias,
		"%r", tokens.user,
		"%l", "",
	)
	return replacer.Replace(value)
}

func isReadableRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// ReadKeyFile reads a private key file discovered through Parse.
//
// Security: the size cap and regular-file check keep a mistaken or hostile
// IdentityFile entry (a device node, a huge file) from stalling the import.
// The caller contract — only paths returned by Parse — is what keeps this from
// being a general file reader; see domain.SSHConfigImporter.
func ReadKeyFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("stat key %s: %w", filepath.Base(path), domain.ErrSSHConfigNotFound)
		}
		return nil, fmt.Errorf("stat key %s: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("key %s is not a regular file: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	if info.Size() > maxKeyFileSize {
		return nil, fmt.Errorf("read key %s: %w", filepath.Base(path), domain.ErrSSHConfigTooLarge)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path originates from a prior Parse of the user's own config; type and size are checked above.
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	return data, nil
}
