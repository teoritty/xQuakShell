package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
)

// ValidateDeclaredUIAssets refuses a plugin tree that is missing a ui/ file its manifest promises.
//
// This is an install-time admission check, not a load-time one, and the distinction is
// deliberate. loadPluginDir runs on every plugin at every startup, and a plugin it rejects never
// reaches the registry — which is also where uninstall looks it up. Failing there would leave a
// user holding a plugin they can neither run nor remove from the UI. Refusing at install instead
// keeps the broken tree out in the first place, and does it before the user is asked to consent
// to anything, while an already-installed one stays visible and removable.
//
// The paths are lexically confined to ui/ by ValidateViewAssetEntry during manifest validation,
// which has already run on every path that reaches here.
//
// Missing discovery icons are separately tolerated at load time by validateDiscoveryIconAssets;
// the two are a pair, not a disagreement — the gate refuses, the loader carries on.
func ValidateDeclaredUIAssets(m *domainplugin.Manifest, dir string) error {
	if dir == "" {
		return nil
	}
	for _, rel := range m.DeclaredUIAssets() {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("%w: plugin %s declares %q", domainplugin.ErrUIAssetMissing, m.ID, rel)
		}
	}
	return nil
}
