package capability

import (
	"fmt"
	"path/filepath"
	"strings"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/pkg/pathsafe"
)

const placeholderPluginData = "${pluginData}"

func resolveRoots(patterns []string, pluginDataDir string) ([]string, error) {
	seen := make(map[string]struct{})
	roots := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		resolved := strings.ReplaceAll(pattern, placeholderPluginData, pluginDataDir)
		abs, err := filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", pattern, err)
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		roots = append(roots, abs)
	}
	return roots, nil
}

// ResolvePath normalizes a plugin-supplied path and verifies it lies under allowed roots.
func ResolvePath(requestPath, pluginDataDir string, readRoots, writeRoots []string, write bool) (string, error) {
	roots := readRoots
	if write {
		roots = writeRoots
	}
	if len(roots) == 0 {
		return "", domainplugin.ErrCapabilityDenied
	}

	candidate := requestPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(pluginDataDir, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve fs path: %w", err)
	}
	abs = filepath.Clean(abs)

	for _, root := range roots {
		if pathsafe.UnderRoot(root, abs) {
			return abs, nil
		}
	}
	return "", domainplugin.ErrCapabilityDenied
}
