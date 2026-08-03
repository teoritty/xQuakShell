package pathsafe

import (
	"fmt"
	"path/filepath"
	"strings"
)

// UnderRoot reports whether target is equal to root or a descendant of root.
func UnderRoot(root, target string) bool {
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
