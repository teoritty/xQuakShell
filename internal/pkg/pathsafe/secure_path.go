package pathsafe

import (
	"os"
	"path/filepath"
	"strings"
)

// SecurePathUnderRoots resolves absPath and verifies it stays within roots without symlink escape.
func SecurePathUnderRoots(absPath string, roots []string) (string, error) {
	absPath = filepath.Clean(absPath)
	if len(roots) == 0 {
		return "", ErrPathDenied
	}

	var matchedRoot string
	for _, root := range roots {
		root = filepath.Clean(root)
		if UnderRoot(root, absPath) {
			matchedRoot = root
			break
		}
	}
	if matchedRoot == "" {
		return "", ErrPathDenied
	}

	rel, err := filepath.Rel(matchedRoot, absPath)
	if err != nil {
		return "", ErrPathDenied
	}
	if rel == "." {
		return matchedRoot, nil
	}

	current := matchedRoot
	segments := strings.Split(rel, string(filepath.Separator))
	for i, seg := range segments {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return "", ErrPathDenied
		}
		current = filepath.Join(current, seg)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return secureNewPath(current, matchedRoot)
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrPathDenied
		}
		if i == len(segments)-1 {
			eval, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			eval = filepath.Clean(eval)
			if !underResolvedRoot(matchedRoot, eval) {
				return "", ErrPathDenied
			}
			return eval, nil
		}
	}
	return matchedRoot, nil
}

func secureNewPath(target, root string) (string, error) {
	parent := filepath.Dir(target)
	if !UnderRoot(root, parent) {
		return "", ErrPathDenied
	}
	evalParent, err := SecurePathUnderRoots(parent, []string{root})
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(evalParent, filepath.Base(target))
	if !underResolvedRoot(root, resolved) {
		return "", ErrPathDenied
	}
	return resolved, nil
}

// underResolvedRoot answers whether a path that has ALREADY been resolved to its canonical form is
// inside root, comparing the two in the same spelling.
//
// The plain comparison is tried first and answers almost every call. It fails on a root and a
// resolved path that name the same directory differently, and on Windows that is not exotic:
// EvalSymlinks returns the canonical long name, so a root handed in as C:\Users\RUNNER~1\... never
// prefixes a target that came back as C:\Users\runneradmin\.... Every user whose profile directory
// has an 8.3 short name - which is every user whose account name is long enough - was refused
// access to their own plugin data, and the CI runner is one of them.
//
// Resolving the root and comparing canonical against canonical is not a loosening. A path whose
// real location is inside the root's real location IS inside the root; the previous comparison was
// simply asking the question in two different alphabets. Everything that made this check a boundary
// is untouched: the per-segment walk above still refuses a symlink anywhere below the root, so a
// link inside the tree cannot lead out of it, and a root that cannot be resolved at all is refused.
func underResolvedRoot(root, resolved string) bool {
	if UnderRoot(root, resolved) {
		return true
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	return UnderRoot(filepath.Clean(rootEval), resolved)
}
