package pathsafe

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// A path that is not valid UTF-8 is refused before anything tries to reason about it, and both
// functions below refuse it in the deny direction.
//
// This is not tidiness. On Windows, filepath.Abs round-trips the string through the OS as UTF-16,
// so an invalid byte comes back as a replacement character: the resolved path no longer names the
// file the caller asked about, and a check run on it vouches for something else. Worse, a fuzz run
// over the sandbox shim's argv found such a path making filepath.Rel spin forever inside
// strings.EqualFold - a hang, in the primitive that decides whether an SFTP path escaped its root.
//
// No legitimate caller is affected: every path this application handles comes from a Go string that
// was already valid UTF-8, or from a filesystem walk that produced one.
var errInvalidUTF8 = fmt.Errorf("path is not valid UTF-8")

// UnderRoot reports whether target is equal to root or a descendant of root.
func UnderRoot(root, target string) bool {
	if !utf8.ValidString(root) || !utf8.ValidString(target) {
		return false
	}
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ResolveUnderRoot resolves rel against root and verifies the result stays under root.
func ResolveUnderRoot(root, rel string) (string, error) {
	if !utf8.ValidString(root) {
		return "", fmt.Errorf("resolve root: %w", errInvalidUTF8)
	}
	if !utf8.ValidString(rel) {
		return "", fmt.Errorf("resolve target: %w", errInvalidUTF8)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	rootAbs = filepath.Clean(rootAbs)

	candidate := rel
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	targetAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve target: %w", err)
	}
	targetAbs = filepath.Clean(targetAbs)

	if !UnderRoot(rootAbs, targetAbs) {
		return "", fmt.Errorf("path escapes root")
	}
	return targetAbs, nil
}
