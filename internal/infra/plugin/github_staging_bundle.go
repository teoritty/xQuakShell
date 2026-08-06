package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/infra/plugin/bundle"
)

// stageBundle unpacks an .xqsp release asset into the staging directory verbatim and returns the
// manifest it contains.
//
// Nothing is written into the tree here — not a regenerated plugin.json, not a regenerated
// SHA256SUMS. The bundle's checksums are the author's statement about its contents, and the
// manifest signature (when there is one) is bound to their digest, so rewriting either turns a
// verifiable bundle into one that only verifies against itself. That tautology is exactly what
// this install path used to produce.
//
// RequireChecksums is called with no reserved names on purpose: at this instant the tree is
// precisely what the author packed. install-meta.json and the user-installed marker are written
// afterwards, and the later validation in loadSource exempts them by name.
func stageBundle(tempDir, bundlePath string) (domainplugin.Manifest, error) {
	if err := bundle.Extract(bundlePath, tempDir); err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("extract bundle: %w", err)
	}
	if err := bundle.RequireChecksums(tempDir); err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("validate bundle checksums: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "plugin.json"))
	if err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("read bundle manifest: %w", err)
	}
	var manifest domainplugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domainplugin.Manifest{}, fmt.Errorf("decode bundle manifest: %w", err)
	}
	return manifest, nil
}
