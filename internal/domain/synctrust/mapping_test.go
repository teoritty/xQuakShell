package synctrust_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

func populatedVault() *domain.VaultData {
	d := domain.NewVaultData()
	d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f1", Name: "Prod", Order: 1})
	d.Connections = append(d.Connections, domain.Connection{ID: "c1", FolderID: "f1", Name: "web", Host: "web.example", Port: 22})
	d.Identities["k1"] = domain.SSHIdentity{ID: "k1", KeyType: "ed25519", AllowForwarding: true}
	d.KeyBlobs["k1"] = domain.IdentityBlob{PEMData: []byte("pem"), DataKey: []byte("dk")}
	d.Passwords["p1"] = domain.PasswordBlob{Value: []byte("secret"), Label: "db"}
	d.PluginSecrets["c1.token"] = []byte("tok")
	d.KnownHosts = append(d.KnownHosts, "web.example ssh-ed25519 AAAA1", "@cert-authority *.example ssh-rsa BBBB")
	d.PeerTrust = append(d.PeerTrust, domain.PeerTrustEntry{Scope: "plug", Subject: "c1", Material: []byte{1, 2}, AddedAt: time.Unix(100, 0).UTC()})
	d.Settings.Terminal.FontSize = 19
	d.Settings.ExternalEditorPath = "local-only"
	return d
}

func asChanges(t *testing.T, snap map[vaultsync.ID]synctrust.Entry) []synctrust.Entry {
	t.Helper()
	out := make([]synctrust.Entry, 0, len(snap))
	for _, e := range snap {
		out = append(out, e)
	}
	return out
}

func snapshot(t *testing.T, d *domain.VaultData) map[vaultsync.ID]synctrust.Entry {
	t.Helper()
	snap, err := synctrust.Snapshot(namingKey, d)
	if err != nil {
		t.Fatal(err)
	}
	out := map[vaultsync.ID]synctrust.Entry{}
	for id, e := range snap {
		want, err := synctrust.RecordID(namingKey, e.Type, e.Key)
		if err != nil || want != id {
			t.Fatalf("%s %q is filed under an id that is not its keyed name", e.Type, e.Key)
		}
		out[id] = e
	}
	return out
}

func TestSnapshotApplyRoundTrip(t *testing.T) {
	src := populatedVault()
	dst := domain.NewVaultData()
	dst.Settings.ExternalEditorPath = "mine"
	if err := synctrust.Apply(dst, asChanges(t, snapshot(t, src))); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]any{
		"folders":       {src.Folders, dst.Folders},
		"connections":   {src.Connections, dst.Connections},
		"identities":    {src.Identities, dst.Identities},
		"keyBlobs":      {src.KeyBlobs, dst.KeyBlobs},
		"passwords":     {src.Passwords, dst.Passwords},
		"pluginSecrets": {src.PluginSecrets, dst.PluginSecrets},
		"peerTrust":     {src.PeerTrust, dst.PeerTrust},
		"terminal":      {src.Settings.Terminal, dst.Settings.Terminal},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s did not survive the round trip:\n got %+v\nwant %+v", name, pair[1], pair[0])
		}
	}
	if len(dst.KnownHosts) != 2 {
		t.Errorf("known hosts = %v", dst.KnownHosts)
	}
	if dst.Settings.ExternalEditorPath != "mine" {
		t.Error("a device-local setting was overwritten by a snapshot from another device")
	}
}

func TestApplyDeletesByNaturalKey(t *testing.T) {
	d := populatedVault()
	err := synctrust.Apply(d, []synctrust.Entry{
		{Type: synctrust.TypeFolder, Key: "f1"},
		{Type: synctrust.TypeConnection, Key: "c1"},
		{Type: synctrust.TypeIdentity, Key: "k1"},
		{Type: synctrust.TypePassword, Key: "p1"},
		{Type: synctrust.TypePluginSecret, Key: "c1.token"},
		{Type: synctrust.TypeKnownHost, Key: "web.example ssh-ed25519"},
		{Type: synctrust.TypePeerTrust, Key: "plug\nc1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Folders)+len(d.Connections)+len(d.Identities)+len(d.KeyBlobs)+len(d.Passwords)+len(d.PluginSecrets)+len(d.PeerTrust) != 0 {
		t.Fatalf("a deletion left something behind: %+v", d)
	}
	if len(d.KnownHosts) != 1 || d.KnownHosts[0] != "@cert-authority *.example ssh-rsa BBBB" {
		t.Fatalf("known-host deletion removed the wrong line: %v", d.KnownHosts)
	}
}

func TestKnownHostUpsertReplacesTheHostsKey(t *testing.T) {
	d := populatedVault()
	data, _ := json.Marshal(map[string]string{"line": "web.example ssh-ed25519 CCCC"})
	if err := synctrust.Apply(d, []synctrust.Entry{{Type: synctrust.TypeKnownHost, Key: "web.example ssh-ed25519", Data: data}}); err != nil {
		t.Fatal(err)
	}
	if len(d.KnownHosts) != 2 || d.KnownHosts[0] != "web.example ssh-ed25519 CCCC" {
		t.Fatalf("known hosts = %v, want the line replaced in place", d.KnownHosts)
	}
}

// A record's id is derived from its natural key, so a body whose own key disagrees with the key it
// was filed under is a device trying to overwrite one item through another item's record.
func TestApplyRefusesAKeyThatDisagreesWithTheData(t *testing.T) {
	src := populatedVault()
	for _, e := range snapshot(t, src) {
		if e.Type == synctrust.SettingsType("terminal") {
			continue
		}
		e.Key = "someone-else"
		d := populatedVault()
		before, _ := json.Marshal(d)
		err := synctrust.Apply(d, []synctrust.Entry{e})
		if !errors.Is(err, synctrust.ErrRecordData) {
			t.Errorf("%s with a foreign key: err = %v, want ErrRecordData", e.Type, err)
		}
		after, _ := json.Marshal(d)
		if string(before) != string(after) {
			t.Errorf("%s: a refused change modified the vault", e.Type)
		}
	}
}

func TestApplyIsAllOrNothing(t *testing.T) {
	d := populatedVault()
	good, _ := json.Marshal(domain.ConnectionFolder{ID: "f2", Name: "Dev"})
	bad, _ := json.Marshal(domain.Connection{ID: "c2", Port: 70000})
	err := synctrust.Apply(d, []synctrust.Entry{
		{Type: synctrust.TypeFolder, Key: "f2", Data: good},
		{Type: synctrust.TypeConnection, Key: "c2", Data: bad},
	})
	if !errors.Is(err, synctrust.ErrRecordData) {
		t.Fatalf("err = %v, want ErrRecordData from the domain validator", err)
	}
	if len(d.Folders) != 1 {
		t.Fatal("a valid change in a refused batch was applied; the vault would hold half a commit")
	}
}

func TestApplyRefusals(t *testing.T) {
	for name, e := range map[string]synctrust.Entry{
		"unknown type":      {Type: "evil", Key: "x", Data: json.RawMessage(`{}`)},
		"settings deletion": {Type: synctrust.SettingsType("terminal"), Key: "terminal"},
		"settings wrong key": {Type: synctrust.SettingsType("terminal"), Key: "lockout",
			Data: json.RawMessage(`{"fontFamily":"x","fontSize":1,"fontColor":"x"}`)},
		"unknown field":   {Type: synctrust.TypeFolder, Key: "f9", Data: json.RawMessage(`{"id":"f9","name":"x","parentId":"","order":0,"evil":1}`)},
		"empty folder id": {Type: synctrust.TypeFolder, Key: "", Data: json.RawMessage(`{"id":"","name":"x","parentId":"","order":0}`)},
		"empty folder name": {Type: synctrust.TypeFolder, Key: "f9",
			Data: json.RawMessage(`{"id":"f9","name":"","parentId":"","order":0}`)},
		"no key in known host":  {Type: synctrust.TypeKnownHost, Key: "web.example ssh-rsa", Data: json.RawMessage(`{"line":"web.example ssh-rsa"}`)},
		"malformed known host":  {Type: synctrust.TypeKnownHost, Key: "web.example", Data: json.RawMessage(`{"line":"web.example"}`)},
		"peer trust no subject": {Type: synctrust.TypePeerTrust, Key: "plug\n", Data: json.RawMessage(`{"scope":"plug","subject":"","material":"AQ==","addedAt":"2026-01-01T00:00:00Z"}`)},
		"vault-meta":            {Type: synctrust.TypeVaultMeta, Key: "", Data: json.RawMessage(`{}`)},
	} {
		d := populatedVault()
		if err := synctrust.Apply(d, []synctrust.Entry{e}); err == nil {
			t.Errorf("%s was applied", name)
		}
	}
}

func TestKnownHostUpdateCollapsesDuplicateLines(t *testing.T) {
	d := domain.NewVaultData()
	d.KnownHosts = []string{"a ssh-rsa K1", "b ssh-rsa X", "a ssh-rsa K2", "a ssh-rsa K3"}
	if n := len(snapshot(t, d)) - 12; n != 2 {
		t.Fatalf("snapshot filed %d known-host records for two hosts; duplicates would share an id", n)
	}
	data, _ := json.Marshal(map[string]string{"line": "a ssh-rsa NEW"})
	if err := synctrust.Apply(d, []synctrust.Entry{{Type: synctrust.TypeKnownHost, Key: "a ssh-rsa", Data: data}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.KnownHosts, []string{"a ssh-rsa NEW", "b ssh-rsa X"}) {
		t.Fatalf("known hosts = %v, want one line per host so every device converges", d.KnownHosts)
	}
}

func TestSnapshotRefusesTwoItemsWithOneKey(t *testing.T) {
	d := domain.NewVaultData()
	d.Folders = []domain.ConnectionFolder{{ID: "f", Name: "a"}, {ID: "f", Name: "b"}}
	if _, err := synctrust.Snapshot(namingKey, d); !errors.Is(err, synctrust.ErrDuplicateKey) {
		t.Fatalf("err = %v, want ErrDuplicateKey; one of the two would silently stop syncing", err)
	}
}

func TestUpsertUpdatesInPlace(t *testing.T) {
	d := populatedVault()
	d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "f2", Name: "Dev"})
	renamed, _ := json.Marshal(domain.ConnectionFolder{ID: "f1", Name: "Production"})
	noBlob, _ := json.Marshal(map[string]any{"identity": domain.SSHIdentity{ID: "k1", KeyType: "ed25519"}})
	if err := synctrust.Apply(d, []synctrust.Entry{
		{Type: synctrust.TypeFolder, Key: "f1", Data: renamed},
		{Type: synctrust.TypeIdentity, Key: "k1", Data: noBlob},
	}); err != nil {
		t.Fatal(err)
	}
	if len(d.Folders) != 2 || d.Folders[0].Name != "Production" {
		t.Fatalf("folders = %+v, want f1 renamed where it was", d.Folders)
	}
	if _, ok := d.KeyBlobs["k1"]; ok {
		t.Fatal("an identity version without a blob left the old private key behind")
	}
}

func TestKnownHostKeys(t *testing.T) {
	d := domain.NewVaultData()
	d.KnownHosts = []string{"@cert-authority *.example ssh-rsa A", "@revoked *.example ssh-rsa B", "h ssh-rsa C"}
	err := synctrust.Apply(d, []synctrust.Entry{{Type: synctrust.TypeKnownHost, Key: "@cert-authority *.example ssh-rsa"}})
	if err != nil || len(d.KnownHosts) != 2 || d.KnownHosts[0] != "@revoked *.example ssh-rsa B" {
		t.Fatalf("known hosts = %v (%v); a marker line's key is marker, pattern and key type", d.KnownHosts, err)
	}
	for _, line := range []string{"h ssh-rsa", "@revoked h ssh-rsa"} {
		data, _ := json.Marshal(map[string]string{"line": line})
		if err := synctrust.Apply(d, []synctrust.Entry{{Type: synctrust.TypeKnownHost, Key: "h ssh-rsa", Data: data}}); err == nil {
			t.Errorf("line %q without a key was applied", line)
		}
	}
}

func TestPeerTrustKeysDoNotCollide(t *testing.T) {
	d := domain.NewVaultData()
	d.PeerTrust = []domain.PeerTrustEntry{{Scope: "a", Subject: "bc"}, {Scope: "ab", Subject: "c"}}
	if _, err := synctrust.Snapshot(namingKey, d); err != nil {
		t.Fatalf("two different scope/subject pairs collided: %v", err)
	}
}

func TestNaturalKeyAgreesWithSnapshot(t *testing.T) {
	for _, e := range snapshot(t, populatedVault()) {
		key, err := synctrust.NaturalKey(e.Type, e.Data)
		if err != nil || key != e.Key {
			t.Errorf("%s: natural key %q, %v; snapshot filed it as %q", e.Type, key, err, e.Key)
		}
	}
	if _, err := synctrust.NaturalKey(synctrust.TypeFolder, json.RawMessage(`{"id":"f","name":"x","parentId":"","order":0,"z":1}`)); err == nil {
		t.Error("a natural key was read from data that does not decode strictly")
	}
	if _, err := synctrust.NaturalKey("evil", json.RawMessage(`{}`)); !errors.Is(err, synctrust.ErrRecordType) {
		t.Errorf("unknown type: %v", err)
	}
}

func TestValidateChecksWithoutApplying(t *testing.T) {
	bad, _ := json.Marshal(domain.Connection{ID: "c9", Port: 70000})
	if err := synctrust.Validate(synctrust.Entry{Type: synctrust.TypeConnection, Key: "c9", Data: bad}); !errors.Is(err, synctrust.ErrRecordData) {
		t.Fatalf("err = %v, want ErrRecordData", err)
	}
	good, _ := json.Marshal(domain.ConnectionFolder{ID: "f9", Name: "x"})
	if err := synctrust.Validate(synctrust.Entry{Type: synctrust.TypeFolder, Key: "f9", Data: good}); err != nil {
		t.Fatal(err)
	}
	if err := synctrust.Validate(synctrust.Entry{Type: synctrust.SettingsType("terminal"), Key: "terminal"}); err == nil {
		t.Fatal("a settings deletion validated")
	}
}

func conflict(itemKey string, path ...string) domain.SyncConflict {
	return domain.SyncConflict{ID: domain.ConflictID("password", itemKey, path), RecordType: "password", ItemKey: itemKey,
		Path: path, Local: json.RawMessage(`"bG9jYWw="`), Remote: json.RawMessage(`"cmVtb3Rl"`), AtUnixMilli: 5}
}

func TestConflictsSyncAsRecords(t *testing.T) {
	src := domain.NewVaultData()
	src.Sync = &domain.VaultSync{Conflicts: []domain.SyncConflict{conflict("p1", "value")}}
	dst := domain.NewVaultData()
	if err := synctrust.Apply(dst, asChanges(t, snapshot(t, src))); err != nil {
		t.Fatal(err)
	}
	if dst.Sync == nil || !reflect.DeepEqual(dst.Sync.Conflicts, src.Sync.Conflicts) {
		t.Fatalf("conflicts = %+v", dst.Sync)
	}
	if err := synctrust.Apply(dst, []synctrust.Entry{{Type: synctrust.TypeConflict, Key: conflict("p1", "value").ID}}); err != nil {
		t.Fatal(err)
	}
	if len(dst.Sync.Conflicts) != 0 {
		t.Fatal("resolving a conflict elsewhere did not remove it here")
	}
}

func TestConflictIDMustMatchItsContent(t *testing.T) {
	c := conflict("p1", "value")
	c.ItemKey = "p2"
	data, _ := json.Marshal(c)
	if err := synctrust.Validate(synctrust.Entry{Type: synctrust.TypeConflict, Key: c.ID, Data: data}); err == nil {
		t.Fatal("a conflict whose id names another item was accepted")
	}
	c = conflict("p1")
	data, _ = json.Marshal(c)
	if err := synctrust.Validate(synctrust.Entry{Type: synctrust.TypeConflict, Key: c.ID, Data: data}); err == nil {
		t.Fatal("a conflict without a field path was accepted")
	}
}

func TestConflictIDsCannotBeForged(t *testing.T) {
	if domain.ConflictID("a", "b\",\"c", nil) == domain.ConflictID("a", "b", []string{"c"}) {
		t.Fatal("an item key containing the separator forged another conflict's id")
	}
}
