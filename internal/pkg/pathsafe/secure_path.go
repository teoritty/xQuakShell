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
			if !UnderRoot(matchedRoot, eval) {
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
	if !UnderRoot(root, resolved) {
		return "", ErrPathDenied
	}
	return resolved, nil
}
