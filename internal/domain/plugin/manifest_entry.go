package plugin

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateBundleRelativePath rejects paths that could escape a plugin bundle root.
// Used for engine.entry and other manifest paths resolved under RootDir.
func ValidateBundleRelativePath(rel string) error {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return fmt.Errorf("%w: bundle path is required", ErrInvalidManifest)
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("%w: bundle path must be relative", ErrInvalidManifest)
	}
	slash := filepath.ToSlash(rel)
	if strings.HasPrefix(slash, "/") {
		return fmt.Errorf("%w: bundle path must be relative", ErrInvalidManifest)
	}
	if len(slash) >= 2 && slash[1] == ':' {
		return fmt.Errorf("%w: bundle path must be relative", ErrInvalidManifest)
	}
	if strings.Contains(rel, "..") {
		return fmt.Errorf("%w: bundle path must not contain ..", ErrInvalidManifest)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || strings.HasPrefix(clean, "..") {
		return fmt.Errorf("%w: invalid bundle path %q", ErrInvalidManifest, rel)
	}
	return nil
}

const viewAssetRootPrefix = "ui/"

// ValidateViewAssetEntry ensures a view entry resolves under the plugin ui/ sandbox.
func ValidateViewAssetEntry(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		entry = "ui/index.html"
	}
	if err := ValidateBundleRelativePath(entry); err != nil {
		return err
	}
	slash := filepath.ToSlash(entry)
	if !strings.HasPrefix(slash, viewAssetRootPrefix) {
		return fmt.Errorf("%w: view entry must be under ui/", ErrInvalidManifest)
	}
	return nil
}
