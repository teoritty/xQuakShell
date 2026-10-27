package synctrust_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/plugin"
	"xquakshell/internal/domain/synctrust"
)

var digest64 = strings.Repeat("ab", 32)

func githubPlugin() domain.SyncPlugin {
	return domain.SyncPlugin{ID: "com.example.echo", Version: "1.2.0",
		Source:         domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: "example/echo", Tag: "v1.2.0"},
		ManifestDigest: digest64, SumsDigest: digest64}
}

func pluginFile(plugin, path string, content []byte) domain.SyncPluginFile {
	sum := sha256.Sum256(content)
	return domain.SyncPluginFile{Plugin: plugin, Path: path, Mode: 0o600, SHA256: hex.EncodeToString(sum[:]), Content: content}
}

func entryOf(t *testing.T, ty synctrust.RecordType, key string, v any) synctrust.Entry {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return synctrust.Entry{Type: ty, Key: key, Data: raw}
}

func TestPluginInstallsSyncInThePersonalVaultOnly(t *testing.T) {
	d := mixedVault()
	d.Sync = &domain.VaultSync{Plugins: []domain.SyncPlugin{githubPlugin()}}
	e, ok := scoped(t, d, synctrust.Scope{})["plugin-install com.example.echo"]
	if !ok {
		t.Fatal("an installed plugin is offered on the user's other devices")
	}
	if len(scoped(t, d, synctrust.Scope{Vault: team})) != len(scoped(t, mixedVault(), synctrust.Scope{Vault: team})) {
		t.Fatal("a team snapshot lists a member's plugins")
	}
	if (synctrust.Scope{Vault: team}).Allows(synctrust.TypePluginInstall) {
		t.Fatal("a team vault allows plugin installs; a member must not plant a plugin for the others")
	}
	dst := domain.NewVaultData()
	if err := synctrust.Apply(dst, []synctrust.Entry{e}); err != nil {
		t.Fatal(err)
	}
	if dst.Sync == nil || len(dst.Sync.Plugins) != 1 || dst.Sync.Plugins[0] != githubPlugin() {
		t.Fatalf("plugins after apply = %+v", dst.Sync)
	}
}

func TestPluginInstallValidation(t *testing.T) {
	refused := map[string]func(*domain.SyncPlugin){
		"bad id":                func(p *domain.SyncPlugin) { p.ID = "../evil" },
		"no version":            func(p *domain.SyncPlugin) { p.Version = "" },
		"unknown source":        func(p *domain.SyncPlugin) { p.Source.Kind = "url" },
		"github without repo":   func(p *domain.SyncPlugin) { p.Source.Repo = "" },
		"github repo not o/n":   func(p *domain.SyncPlugin) { p.Source.Repo = "https://evil.example/x" },
		"github without tag":    func(p *domain.SyncPlugin) { p.Source.Tag = "" },
		"github without sums":   func(p *domain.SyncPlugin) { p.SumsDigest = "" },
		"short manifest digest": func(p *domain.SyncPlugin) { p.ManifestDigest = "ab" },
		"non-hex sums digest":   func(p *domain.SyncPlugin) { p.SumsDigest = strings.Repeat("zz", 32) },
		"manual with a repo": func(p *domain.SyncPlugin) {
			p.Source = domain.SyncPluginSource{Kind: domain.PluginSourceManual, Repo: "a/b"}
		},
		"control in publisher": func(p *domain.SyncPlugin) { p.PublisherKey = "key\n" },
	}
	for name, edit := range refused {
		p := githubPlugin()
		edit(&p)
		if err := synctrust.Validate(entryOf(t, synctrust.TypePluginInstall, p.ID, p)); !errors.Is(err, synctrust.ErrRecordData) {
			t.Errorf("%s: err = %v, want ErrRecordData", name, err)
		}
	}
	manual := domain.SyncPlugin{ID: "com.example.local", Version: "0.1.0", Source: domain.SyncPluginSource{Kind: domain.PluginSourceManual},
		ManifestDigest: digest64}
	if err := synctrust.Validate(entryOf(t, synctrust.TypePluginInstall, manual.ID, manual)); err != nil {
		t.Fatalf("a manual install without a SHA256SUMS: %v", err)
	}
}

func TestPluginFilesSyncInThePersonalVaultOnly(t *testing.T) {
	d := mixedVault()
	f := pluginFile("com.example.echo", "state/bookmarks.json", []byte(`{"a":1}`))
	d.Sync = &domain.VaultSync{PluginFiles: []domain.SyncPluginFile{f}}
	e, ok := scoped(t, d, synctrust.Scope{})["plugin-data com.example.echo\nstate/bookmarks.json"]
	if !ok {
		t.Fatalf("personal snapshot = %v; a declared plugin file follows the user", keys(scoped(t, d, synctrust.Scope{})))
	}
	if (synctrust.Scope{Vault: team}).Allows(synctrust.TypePluginData) {
		t.Fatal("a team vault allows plugin data")
	}
	dst := domain.NewVaultData()
	if err := synctrust.Apply(dst, []synctrust.Entry{e}); err != nil {
		t.Fatal(err)
	}
	if got := dst.Sync.PluginFiles; len(got) != 1 || string(got[0].Content) != `{"a":1}` || got[0].Path != f.Path {
		t.Fatalf("plugin files after apply = %+v", got)
	}
	if err := synctrust.Apply(dst, []synctrust.Entry{{Type: synctrust.TypePluginData, Key: e.Key}}); err != nil || len(dst.Sync.PluginFiles) != 0 {
		t.Fatalf("a deleted plugin file stays: %v, %+v", err, dst.Sync.PluginFiles)
	}
}

func TestPluginFileValidationRefusesAHostileDevice(t *testing.T) {
	refused := map[string]domain.SyncPluginFile{
		"traversal":  pluginFile("com.example.echo", "../vault.age", []byte("x")),
		"absolute":   pluginFile("com.example.echo", "/etc/passwd", []byte("x")),
		"drive":      pluginFile("com.example.echo", "C:/Windows/x", []byte("x")),
		"ads":        pluginFile("com.example.echo", "state.json:evil", []byte("x")),
		"tmp":        pluginFile("com.example.echo", "tmp/x", []byte("x")),
		"bad plugin": pluginFile("../other", "state.json", []byte("x")),
		"oversize":   pluginFile("com.example.echo", "big.bin", make([]byte, plugin.MaxSyncReceivedFileBytes+1)),
		"executable mode": func() domain.SyncPluginFile {
			f := pluginFile("com.example.echo", "run.sh", []byte("x"))
			f.Mode = 0o755
			return f
		}(),
		"wrong digest": func() domain.SyncPluginFile {
			f := pluginFile("com.example.echo", "a", []byte("x"))
			f.SHA256 = digest64
			return f
		}(),
	}
	for name, f := range refused {
		e := entryOf(t, synctrust.TypePluginData, f.Plugin+"\n"+f.Path, f)
		if err := synctrust.Validate(e); !errors.Is(err, synctrust.ErrRecordData) {
			t.Errorf("%s: err = %v, want ErrRecordData", name, err)
		}
	}
	// The limit on what arrives is the older, larger one, so a file an older build sent between the
	// two limits is applied, not quarantined.
	at := pluginFile("com.example.echo", "exactly.bin", make([]byte, plugin.MaxSyncReceivedFileBytes))
	if err := synctrust.Validate(entryOf(t, synctrust.TypePluginData, at.Plugin+"\n"+at.Path, at)); err != nil {
		t.Fatalf("a file of exactly the received limit: %v", err)
	}
}

func TestPluginTypesAreKnownSchemas(t *testing.T) {
	for _, ty := range []synctrust.RecordType{synctrust.TypePluginInstall, synctrust.TypePluginData} {
		if synctrust.KnownSchemas()[string(ty)] != 1 {
			t.Errorf("%s is not a known schema-1 type", ty)
		}
	}
}

func TestCloneVaultDataCopiesPluginFileContent(t *testing.T) {
	d := domain.NewVaultData()
	d.Sync = &domain.VaultSync{Plugins: []domain.SyncPlugin{githubPlugin()},
		PluginFiles: []domain.SyncPluginFile{pluginFile("com.example.echo", "a", []byte("abc"))}}
	c := domain.CloneVaultData(d)
	c.Sync.PluginFiles[0].Content[0] = 'z'
	if string(d.Sync.PluginFiles[0].Content) != "abc" || len(c.Sync.Plugins) != 1 {
		t.Fatal("a clone shares plugin file bytes with the original, or drops the plugin list")
	}
}
