package portable

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Paths resolves portable application directories next to the executable (ADR-006).
type Paths struct {
	exeDir string
}

// NewPaths returns portable paths based on the current executable location.
func NewPaths() *Paths {
	exe, err := os.Executable()
	if err != nil {
		return &Paths{exeDir: "."}
	}
	return &Paths{exeDir: filepath.Dir(exe)}
}

// ExeDir returns the directory containing the executable.
func (p *Paths) ExeDir() string {
	return p.exeDir
}

// DataRoot returns <exe>/data — root for all application state.
func (p *Paths) DataRoot() string {
	return filepath.Join(p.exeDir, "data")
}

// VaultDir returns the directory holding vault.age (same as DataRoot).
func (p *Paths) VaultDir() string {
	return p.DataRoot()
}

// PluginsDir returns <exe>/data/plugins.
func (p *Paths) PluginsDir() string {
	return filepath.Join(p.DataRoot(), "plugins")
}

// PluginDataDir returns writable storage for a plugin instance.
func (p *Paths) PluginDataDir(pluginID string) string {
	safeID := strings.ReplaceAll(pluginID, string(filepath.Separator), "_")
	return filepath.Join(p.PluginsDir(), safeID, "data")
}

// TempDir returns <exe>/data/tmp for ephemeral portable temp files.
func (p *Paths) TempDir() string {
	return filepath.Join(p.DataRoot(), "tmp")
}

// EnsureDirs creates data root and standard subdirectories.
func (p *Paths) EnsureDirs() error {
	for _, dir := range []string{p.DataRoot(), p.PluginsDir(), p.TempDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			if errors.Is(err, syscall.EROFS) {
				continue
			}
			return err
		}
	}
	return nil
}

// Default is the process-wide portable paths instance.
var Default = NewPaths()
