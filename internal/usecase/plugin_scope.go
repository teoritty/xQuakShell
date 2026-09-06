package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// EnsureScopeRoot gives a plugin that declares a scope one folder and one root, and reports whether
// it made them (ADR-022).
//
// The folder is created by the core and named after the plugin. A plugin that could create or name
// its own would draw a second folder resembling the first, and personal connections would be dropped
// into it by the user's own hand.
//
// It runs on every unlock and does nothing when the plugin already has a scope: a second folder
// would give the user two places to put things and the plugin two places to read from.
func (s *PluginVaultSettings) EnsureScopeRoot(
	ctx context.Context,
	manifest *domainplugin.Manifest,
) (bool, error) {
	if s == nil || s.vault == nil || manifest == nil || manifest.ID == "" || !manifest.NeedsScope() {
		return false, nil
	}
	var created bool
	err := s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.Settings == nil {
			data.Settings = &domain.AppSettings{}
		}
		if hasScopeRoot(data.Settings.Plugins.ScopeRoots, manifest.ID) {
			return nil
		}
		folderID, err := newScopeFolderID()
		if err != nil {
			return err
		}
		data.Folders = append(data.Folders, domain.ConnectionFolder{
			ID:   folderID,
			Name: scopeFolderName(manifest),
		})
		data.Settings.Plugins.ScopeRoots = append(data.Settings.Plugins.ScopeRoots,
			domain.ScopeRoot{FolderID: folderID, PluginID: manifest.ID})
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return created, nil
}

// EnsureScopeRootsForAll gives every installed plugin that declares a scope one, and reports how
// many it made. One plugin's failure does not cost the others theirs, for the same reason it does
// not when consent is recorded: this runs once per unlock.
func (s *PluginVaultSettings) EnsureScopeRootsForAll(
	ctx context.Context,
	installed []domainplugin.InstalledPlugin,
) (int, error) {
	var created int
	var firstErr error
	for i := range installed {
		made, err := s.EnsureScopeRoot(ctx, &installed[i].Manifest)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if made {
			created++
		}
	}
	return created, firstErr
}

func hasScopeRoot(roots []domain.ScopeRoot, pluginID string) bool {
	for _, root := range roots {
		if root.PluginID == pluginID {
			return true
		}
	}
	return false
}

func scopeFolderName(manifest *domainplugin.Manifest) string {
	if manifest.Name != "" {
		return manifest.Name
	}
	return manifest.ID
}

// newScopeFolderID mints a fresh id rather than deriving one from the plugin id.
//
// A derived id would be reused on reinstall, and whatever the user had left in the old folder would
// be exposed again the moment anything was installed under that id - with nobody asked. A plugin id
// is chosen by its author and verified against nothing, so "the same id" is not a coincidence an
// attacker has to wait for.
func newScopeFolderID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint scope folder id: %w", err)
	}
	return "scope-" + hex.EncodeToString(raw), nil
}
