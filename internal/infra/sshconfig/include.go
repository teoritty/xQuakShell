package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"xquakshell/internal/domain"
)

// Parser limits. They exist to bound the work a single Parse call can be made
// to do: an ssh_config is user-authored, but Include accepts globs and can
// reference files that include each other, so an unbounded reader is a denial
// of service against our own process.
const (
	maxIncludeDepth = 8
	maxIncludeFiles = 64
	maxFileSize     = 1 << 20   // 1 MiB per config file
	maxGlobMatches  = 64        // per Include directive
	maxKeyFileSize  = 256 << 10 // 256 KiB per private key
)

// loader expands a root config file and its Include directives into a single
// ordered directive stream, collecting non-fatal findings as it goes.
type loader struct {
	homeDir string
	// visited guards against include cycles; keys are cleaned absolute paths.
	visited map[string]bool
	// filesRead counts every file opened, cycle-free chains included, so a
	// wide include fan-out is bounded as well as a deep one.
	filesRead int
	notices   *noticeSet
}

func newLoader(homeDir string, notices *noticeSet) *loader {
	return &loader{homeDir: homeDir, visited: map[string]bool{}, notices: notices}
}

// load reads path and returns its directives with all Include directives
// expanded in place, matching OpenSSH's textual inclusion semantics.
func (l *loader) load(path string) ([]directive, error) {
	content, err := readTextFile(path)
	if err != nil {
		return nil, err
	}
	abs := cleanAbs(path)
	l.visited[abs] = true
	l.filesRead++
	return l.expand(lexFile(content), filepath.Dir(abs), 0), nil
}

// expand walks a directive stream, replacing include directives with the
// directives of the files they name.
func (l *loader) expand(directives []directive, baseDir string, depth int) []directive {
	out := make([]directive, 0, len(directives))
	for _, d := range directives {
		if d.keyword != "include" {
			out = append(out, d)
			continue
		}
		out = append(out, l.expandInclude(d, baseDir, depth)...)
	}
	return out
}

// expandInclude resolves one Include directive's arguments into directives.
func (l *loader) expandInclude(d directive, baseDir string, depth int) []directive {
	if depth >= maxIncludeDepth {
		l.notices.add(domain.SSHConfigNoticeLimitReached, includeLabel(d))
		return nil
	}
	var out []directive
	for _, arg := range d.args {
		for _, match := range l.resolveIncludeArg(arg, baseDir) {
			out = append(out, l.loadIncluded(match, depth)...)
		}
	}
	return out
}

// loadIncluded reads one already-resolved include target.
func (l *loader) loadIncluded(path string, depth int) []directive {
	abs := cleanAbs(path)
	if l.visited[abs] {
		// A cycle, or the same file pulled in twice. Either way re-reading it
		// would only duplicate directives that first-wins resolution ignores.
		return nil
	}
	if l.filesRead >= maxIncludeFiles {
		l.notices.add(domain.SSHConfigNoticeLimitReached, filepath.Base(abs))
		return nil
	}
	content, err := readTextFile(abs)
	if err != nil {
		l.notices.add(domain.SSHConfigNoticeIncludeUnreadable, filepath.Base(abs))
		return nil
	}
	l.visited[abs] = true
	l.filesRead++
	return l.expand(lexFile(content), filepath.Dir(abs), depth+1)
}

// resolveIncludeArg turns one Include argument into concrete file paths.
//
// OpenSSH resolves relative include paths against ~/.ssh. We additionally try
// the directory of the file doing the including first, because the user may
// have pointed us at a config kept somewhere other than ~/.ssh (a copy under
// review, a dotfiles checkout); both candidates are ordinary user-owned
// locations, so this widens convenience without widening trust.
func (l *loader) resolveIncludeArg(arg, baseDir string) []string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil
	}
	for _, candidate := range l.includeCandidates(arg, baseDir) {
		if matches := globExisting(candidate); len(matches) > 0 {
			return matches
		}
	}
	l.notices.add(domain.SSHConfigNoticeIncludeUnreadable, filepath.Base(arg))
	return nil
}

// includeCandidates lists the paths an Include argument may refer to, in
// priority order.
func (l *loader) includeCandidates(arg, baseDir string) []string {
	expanded := expandTilde(arg, l.homeDir)
	if filepath.IsAbs(expanded) || expanded != arg {
		return []string{expanded}
	}
	candidates := []string{filepath.Join(baseDir, arg)}
	if l.homeDir != "" {
		candidates = append(candidates, filepath.Join(l.homeDir, ".ssh", arg))
	}
	return candidates
}

// globExisting expands a possibly-glob path into existing regular files,
// sorted for deterministic ordering and capped by maxGlobMatches.
func globExisting(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		// Only ErrBadPattern is possible; treat it as "matched nothing".
		return nil
	}
	files := make([]string, 0, len(matches))
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	if len(files) > maxGlobMatches {
		files = files[:maxGlobMatches]
	}
	return files
}

// readTextFile reads a configuration file, refusing anything that is not a
// reasonably sized regular file.
func readTextFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", filepath.Base(path), domain.ErrSSHConfigNotFound)
		}
		return "", fmt.Errorf("stat %s: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file %s: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), domain.ErrSSHConfigTooLarge)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is user-selected or Include-derived; size and type are checked above.
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), domain.ErrSSHConfigUnreadable)
	}
	return string(data), nil
}

// includeLabel names an Include directive for a notice without exposing a
// full path.
func includeLabel(d directive) string {
	if len(d.args) == 0 {
		return "Include"
	}
	return filepath.Base(d.args[0])
}

func cleanAbs(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}
