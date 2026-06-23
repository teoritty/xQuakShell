package plugin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	domainplugin "ssh-client/internal/domain/plugin"
)

// CopyBundle copies a plugin directory tree to dest.
func CopyBundle(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// InstallBundle copies sourceDir into dataRoot/plugins/<id>/ and reloads it.
func InstallBundle(sourceDir, dataRoot string) (domainplugin.InstalledPlugin, error) {
	plugin, err := LoadPluginDir(sourceDir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("load plugin: %w", err)
	}
	destDir, err := SafePluginInstallDir(dataRoot, plugin.Manifest.ID)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	if err := os.RemoveAll(destDir); err != nil {
		return domainplugin.InstalledPlugin{}, fmt.Errorf("prepare install dir: %w", err)
	}
	if err := CopyBundle(sourceDir, destDir); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed, err := LoadPluginDir(destDir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed.Source = domainplugin.SourceUser
	return installed, nil
}
