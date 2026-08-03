package assets

import (
	"path/filepath"
	"strings"
)

var allowedAssetExtensions = map[string]struct{}{
	".html":  {},
	".htm":   {},
	".js":    {},
	".css":   {},
	".svg":   {},
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".webp":  {},
	".woff":  {},
	".woff2": {},
	".ttf":   {},
	".map":   {},
	".json":  {},
	".ico":   {},
}

var blockedAssetBasenames = map[string]struct{}{
	"plugin.json": {},
}

// IsAllowedAssetName reports whether a file name may be served from the UI sandbox.
func IsAllowedAssetName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	if _, blocked := blockedAssetBasenames[lower]; blocked {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	if ext == ".exe" || ext == ".dll" || ext == ".so" || ext == ".dylib" {
		return false
	}
	_, ok := allowedAssetExtensions[ext]
	return ok
}

// ResolveUIRelPath normalizes a request path relative to the plugin ui/ root.
func ResolveUIRelPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errInvalidAssetPath
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." || strings.HasPrefix(rel, "..") || strings.Contains(rel, ".."+string(filepath.Separator)) {
		return "", errInvalidAssetPath
	}
	if strings.HasPrefix(filepath.ToSlash(rel), "ui/") {
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "ui/")
		rel = filepath.FromSlash(rel)
		rel = filepath.Clean(rel)
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", errInvalidAssetPath
	}
	return rel, nil
}
