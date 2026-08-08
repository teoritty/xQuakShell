package plugin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	domainplugin "xquakshell/internal/domain/plugin"
)

// CopyBundle copies a plugin directory tree to dest, landing every file at 0600 except the engine
// entry, which lands at 0700 because the host has to exec it.
//
// entry is the manifest's engine.entry, and it is a parameter rather than something the caller
// chmods afterwards because that is precisely how this broke. github_staging.go does chmod its
// staged binary 0700 — and then this copy, the step that moves a plugin into its final home, wrote
// it back to 0600 and undid that. Windows has no execute bit and ignores the mode entirely, so
// every install path looked correct there for as long as nobody started a plugin on Linux. There
// the symptom is a bare `fork/exec …: permission denied` out of the OS, several layers away from
// the install that caused it, and it takes down every plugin rather than one.
//
// Both name candidates are marked. One manifest is published for every platform and can only name
// the binary once, so a bundle built for Windows carries `xqs-vnc.exe` where the manifest says
// `xqs-vnc`; discovery already accepts that pair, and so must this.
func CopyBundle(src, dest, entry string) error {
	executable := executableEntryNames(entry)

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
		mode := os.FileMode(0o600)
		if executable[filepath.Clean(rel)] {
			mode = 0o700
		}
		return copyFile(path, target, mode)
	})
}

// executableEntryNames returns the set of bundle-relative paths that must be installed with the
// execute bit, keyed the way filepath.Rel reports them so a caller can look up a walked path.
//
// Separate from CopyBundle so the naming rule can be asserted on any platform: the mode itself is
// only observable where an execute bit exists, and Windows — where this bug survived — is not such
// a platform.
func executableEntryNames(entry string) map[string]bool {
	names := make(map[string]bool, 2)
	for _, name := range entryNameCandidates(entry) {
		names[filepath.Clean(filepath.FromSlash(name))] = true
	}
	return names
}

func copyFile(src, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// #nosec G302 -- mode is 0600 for every file except the plugin's engine entry, which must
	// carry the execute bit or the host cannot start it. 0700 keeps that owner-only.
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
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
	if err := CopyBundle(sourceDir, destDir, plugin.Manifest.Engine.Entry); err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed, err := LoadPluginDir(destDir)
	if err != nil {
		return domainplugin.InstalledPlugin{}, err
	}
	installed.Source = domainplugin.SourceUser
	return installed, nil
}
