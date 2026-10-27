package synctrust

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/plugin"
)

// githubRepo is "owner/name" as GitHub allows it. A record naming anything else - a URL, a path -
// would let a device steer another device's install to a host of its choosing.
var githubRepo = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

// pluginInstallMapper syncs the plugins installed on the user's devices. Applying one records it;
// installing it is the user's choice on each device, through the normal install pipeline.
var pluginInstallMapper = sliceMapper(
	func(d *domain.VaultData) *[]domain.SyncPlugin {
		if d.Sync == nil {
			d.Sync = &domain.VaultSync{}
		}
		return &d.Sync.Plugins
	},
	func(p domain.SyncPlugin) string { return p.ID },
	validateSyncPlugin,
)

func validateSyncPlugin(p *domain.SyncPlugin) error {
	if err := plugin.ValidateID(p.ID); err != nil {
		return err
	}
	if !printable(p.Version, 64) || p.Version == "" || !printable(p.PublisherKey, 256) {
		return errors.New("a plugin version or publisher key that is empty, too long or not printable")
	}
	if !isDigest(p.ManifestDigest) || (p.SumsDigest != "" && !isDigest(p.SumsDigest)) {
		return errors.New("a plugin digest that is not hex SHA-256")
	}
	switch s := p.Source; s.Kind {
	case domain.PluginSourceGitHub:
		if !githubRepo.MatchString(s.Repo) || s.Tag == "" || !printable(s.Tag, 128) || p.SumsDigest == "" {
			return errors.New("a GitHub plugin needs owner/name, a tag and a SHA256SUMS digest")
		}
	case domain.PluginSourceManual:
		if s.Repo != "" || s.Tag != "" {
			return errors.New("a manual plugin names no repository")
		}
	default:
		return errors.New("an unknown plugin source")
	}
	return nil
}

// pluginDataMapper syncs the files plugins declared. Which files a plugin declared is checked where
// its manifest is known, when the file is written to disk; here every file must at least be a safe
// path of a valid plugin, within the size limit, not executable, and match its own digest.
var pluginDataMapper = sliceMapper(
	func(d *domain.VaultData) *[]domain.SyncPluginFile {
		if d.Sync == nil {
			d.Sync = &domain.VaultSync{}
		}
		return &d.Sync.PluginFiles
	},
	func(f domain.SyncPluginFile) string { return f.Key() },
	validateSyncPluginFile,
)

func validateSyncPluginFile(f *domain.SyncPluginFile) error {
	if err := plugin.ValidateID(f.Plugin); err != nil {
		return err
	}
	if err := plugin.ValidSyncFile(f.Path); err != nil {
		return err
	}
	if len(f.Content) > plugin.MaxSyncReceivedFileBytes {
		return errors.New("a plugin file over the size limit")
	}
	// Owner read and write, optionally read for others; never executable, never writable by others,
	// so a synced file cannot become a script another device runs.
	if f.Mode&^0o644 != 0 || f.Mode&0o600 != 0o600 {
		return errors.New("a plugin file mode other than 0600 or 0644")
	}
	sum := sha256.Sum256(f.Content)
	if f.SHA256 != hex.EncodeToString(sum[:]) {
		return errors.New("a plugin file that does not match its digest")
	}
	return nil
}

func isDigest(s string) bool {
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == sha256.Size && s == strings.ToLower(s)
}

func printable(s string, maxLen int) bool {
	return len(s) <= maxLen && strings.IndexFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) < 0
}
