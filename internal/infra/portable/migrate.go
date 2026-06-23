package portable

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// MigrateLegacyLayout moves pre-ADR-006 files from <exe>/ into <exe>/data/.
func MigrateLegacyLayout(p *Paths) error {
	if p == nil {
		p = Default
	}
	exeDir := p.ExeDir()
	dataRoot := p.DataRoot()

	newVault := filepath.Join(dataRoot, "vault.age")
	oldVault := filepath.Join(exeDir, "vault.age")
	if fileExists(oldVault) && !fileExists(newVault) {
		if err := os.MkdirAll(dataRoot, 0o700); err != nil {
			return fmt.Errorf("migrate: create data dir: %w", err)
		}
		if err := os.Rename(oldVault, newVault); err != nil {
			return fmt.Errorf("migrate vault.age: %w", err)
		}
		slog.Info("portable migration: moved vault.age to data/")
	}

	for _, name := range []string{"vault.age.tmp", "audit.db", "audit.db-wal", "audit.db-shm"} {
		oldPath := filepath.Join(exeDir, name)
		newPath := filepath.Join(dataRoot, name)
		if fileExists(oldPath) && !fileExists(newPath) {
			if err := os.MkdirAll(dataRoot, 0o700); err != nil {
				return fmt.Errorf("migrate: create data dir: %w", err)
			}
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("migrate %s: %w", name, err)
			}
			slog.Info("portable migration: moved file to data/", "file", name)
		}
	}

	oldPlugins := filepath.Join(exeDir, "plugins")
	newPlugins := p.PluginsDir()
	if dirExists(oldPlugins) && !dirExists(newPlugins) {
		if err := os.MkdirAll(dataRoot, 0o700); err != nil {
			return fmt.Errorf("migrate: create data dir: %w", err)
		}
		if err := os.Rename(oldPlugins, newPlugins); err != nil {
			return fmt.Errorf("migrate plugins: %w", err)
		}
		slog.Info("portable migration: moved plugins/ to data/plugins/")
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
