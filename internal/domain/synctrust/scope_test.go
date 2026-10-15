package synctrust_test

import (
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

const team = "7e57"

// mixedVault holds a personal and a team copy of every shareable item type.
func mixedVault() *domain.VaultData {
	d := populatedVault()
	d.Folders = append(d.Folders, domain.ConnectionFolder{ID: "tf", Name: "Team", VaultID: team})
	d.Connections = append(d.Connections, domain.Connection{ID: "tc", FolderID: "tf", Name: "db", Host: "db.example", Port: 22,
		VaultID: team, PluginFields: map[string]string{"token": "tc.token"}})
	d.Identities["tk"] = domain.SSHIdentity{ID: "tk", KeyType: "ed25519", VaultID: team}
	d.KeyBlobs["tk"] = domain.IdentityBlob{PEMData: []byte("team-pem")}
	d.Passwords["tp"] = domain.PasswordBlob{Value: []byte("team-secret"), Label: "team", VaultID: team}
	d.PluginSecrets["tc.token"] = []byte("team-tok")
	return d
}

func scoped(t *testing.T, d *domain.VaultData, s synctrust.Scope) map[string]synctrust.Entry {
	t.Helper()
	snap, err := synctrust.SnapshotScope(namingKey, d, s)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]synctrust.Entry{}
	for _, e := range snap {
		out[string(e.Type)+" "+e.Key] = e
	}
	return out
}

func TestTeamScopeSnapshotsOnlyItsOwnItems(t *testing.T) {
	got := scoped(t, mixedVault(), synctrust.Scope{Vault: team})
	for _, want := range []string{"folder tf", "connection tc", "identity tk", "password tp"} {
		if _, ok := got[want]; !ok {
			t.Errorf("team snapshot lacks %s", want)
		}
	}
	if len(got) != 4 {
		t.Fatalf("team snapshot = %d entries, want exactly the four team items: %v", len(got), keys(got))
	}
	if _, ok := got["plugin-secret tc.token"]; ok {
		t.Error("team snapshot carries the plugin secret of a team connection; every member would receive this account's credential")
	}
	for k, e := range got {
		if strings.Contains(string(e.Data), "vaultId") {
			t.Errorf("%s carries its vault id in the record data; the envelope's vault is the only one", k)
		}
	}
}

func TestPersonalScopeLeavesTeamItemsOut(t *testing.T) {
	got := scoped(t, mixedVault(), synctrust.Scope{})
	for _, unwanted := range []string{"folder tf", "connection tc", "identity tk", "password tp"} {
		if _, ok := got[unwanted]; ok {
			t.Errorf("personal snapshot holds the team item %s", unwanted)
		}
	}
	if _, ok := got["folder f1"]; !ok {
		t.Fatal("personal snapshot lost a personal folder")
	}
	if _, ok := got["plugin-secret tc.token"]; !ok {
		t.Error("personal snapshot lacks this account's plugin secret for a team connection; it would not reach the account's other devices")
	}
}

func TestATeamScopeDropsPluginSecretsAnOlderBuildShared(t *testing.T) {
	d := mixedVault()
	plant := synctrust.Entry{Type: synctrust.TypePluginSecret, Key: "tc.token", Data: []byte(`{"key":"tc.token","value":"b3RoZXI="}`)}
	wipe := synctrust.Entry{Type: synctrust.TypePluginSecret, Key: "tc.token"}
	for _, c := range []synctrust.Entry{plant, wipe} {
		if err := synctrust.ApplyScope(d, synctrust.Scope{Vault: team}, []synctrust.Entry{c}); err != nil {
			t.Fatalf("a team commit carrying a plugin secret was refused (%v); the rest of that commit would never apply", err)
		}
		if string(d.PluginSecrets["tc.token"]) != "team-tok" {
			t.Fatalf("a team record changed this account's plugin secret to %q", d.PluginSecrets["tc.token"])
		}
	}
}

func TestThePersonalScopeKeepsThePluginSecretOfATeamConnection(t *testing.T) {
	d := mixedVault()
	own := synctrust.Entry{Type: synctrust.TypePluginSecret, Key: "tc.token", Data: []byte(`{"key":"tc.token","value":"bWluZQ=="}`)}
	if err := synctrust.ApplyScope(d, synctrust.Scope{}, []synctrust.Entry{own}); err != nil {
		t.Fatalf("err = %v; this account's own secret for a team connection syncs through the personal vault", err)
	}
	if string(d.PluginSecrets["tc.token"]) != "mine" {
		t.Fatalf("secret = %q, want the value from this account's other device", d.PluginSecrets["tc.token"])
	}
}

func TestApplyIntoATeamScopeStampsTheVault(t *testing.T) {
	src := mixedVault()
	snap, err := synctrust.SnapshotScope(namingKey, src, synctrust.Scope{Vault: team})
	if err != nil {
		t.Fatal(err)
	}
	changes := asChanges(t, snap)
	dst := domain.NewVaultData()
	if err := synctrust.ApplyScope(dst, synctrust.Scope{Vault: team}, changes); err != nil {
		t.Fatal(err)
	}
	if len(dst.Folders) != 1 || dst.Folders[0].VaultID != team || len(dst.Connections) != 1 || dst.Connections[0].VaultID != team {
		t.Fatalf("folders %+v connections %+v; applied team items must carry the team vault", dst.Folders, dst.Connections)
	}
	if dst.Identities["tk"].VaultID != team || dst.Passwords["tp"].VaultID != team {
		t.Fatal("an applied team identity or password lost its vault")
	}
	if len(dst.PluginSecrets) != 0 {
		t.Fatalf("plugin secrets %v arrived through a team vault", dst.PluginSecrets)
	}
}

func TestATeamScopeRefusesPersonalOnlyTypes(t *testing.T) {
	s := synctrust.Scope{Vault: team}
	for _, ty := range []synctrust.RecordType{synctrust.TypeKnownHost, synctrust.TypePeerTrust, synctrust.SettingsType("terminal")} {
		if s.Allows(ty) {
			t.Errorf("a team vault allows %s; members must not plant host keys or settings for each other", ty)
		}
	}
	if !s.Allows(synctrust.TypeConnection) || !(synctrust.Scope{}).Allows(synctrust.TypeKnownHost) {
		t.Fatal("scope allowances are inverted")
	}
	err := synctrust.ApplyScope(domain.NewVaultData(), s, []synctrust.Entry{{Type: synctrust.TypeKnownHost, Key: "h ssh-ed25519",
		Data: []byte(`{"line":"h ssh-ed25519 AAAA"}`)}})
	if !errors.Is(err, synctrust.ErrRecordType) {
		t.Fatalf("err = %v, want ErrRecordType", err)
	}
}

func TestApplyRefusesToTakeAnItemFromAnotherVault(t *testing.T) {
	d := mixedVault()
	steal := synctrust.Entry{Type: synctrust.TypeConnection, Key: "tc",
		Data: []byte(`{"id":"tc","folderId":"","name":"stolen","host":"evil.example","port":22,"order":0}`)}
	if err := synctrust.ApplyScope(d, synctrust.Scope{}, []synctrust.Entry{steal}); !errors.Is(err, synctrust.ErrForeignVault) {
		t.Fatalf("err = %v, want ErrForeignVault: a personal record must not overwrite a team item", err)
	}
	drop := synctrust.Entry{Type: synctrust.TypePassword, Key: "tp"}
	if err := synctrust.ApplyScope(d, synctrust.Scope{}, []synctrust.Entry{drop}); !errors.Is(err, synctrust.ErrForeignVault) {
		t.Fatalf("err = %v, want ErrForeignVault: a personal deletion must not remove a team item", err)
	}
	if d.Connections[len(d.Connections)-1].Name != "db" || d.Passwords["tp"].Label != "team" {
		t.Fatal("a refused change touched the team items")
	}
}

func keys(m map[string]synctrust.Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestConflictsBelongToTheirVault(t *testing.T) {
	d := mixedVault()
	personal := domain.SyncConflict{RecordType: "connection", ItemKey: "c1", Path: []string{"host"}}
	personal.ID = domain.ConflictID(personal.RecordType, personal.ItemKey, personal.Path)
	teamConflict := domain.SyncConflict{RecordType: "connection", ItemKey: "tc", Path: []string{"host"}, VaultID: team}
	teamConflict.ID = domain.ConflictID(teamConflict.RecordType, teamConflict.ItemKey, teamConflict.Path)
	d.Sync = &domain.VaultSync{Conflicts: []domain.SyncConflict{personal, teamConflict}}
	if _, ok := scoped(t, d, synctrust.Scope{})["conflict "+teamConflict.ID]; ok {
		t.Fatal("the personal vault syncs a team conflict")
	}
	if _, ok := scoped(t, d, synctrust.Scope{Vault: team})["conflict "+teamConflict.ID]; !ok {
		t.Fatal("the team vault does not sync its own conflict")
	}
}

func TestContactsSyncInThePersonalVaultOnly(t *testing.T) {
	d := mixedVault()
	digest := strings.Repeat("ab", 32)
	d.Sync = &domain.VaultSync{Contacts: []domain.SyncContact{{Account: strings.Repeat("01", 16), IdentityDigest: digest, Verified: true}}}
	personal := scoped(t, d, synctrust.Scope{})
	e, ok := personal["contact "+strings.Repeat("01", 16)]
	if !ok {
		t.Fatalf("personal snapshot = %v; a contact follows the user to every device", keys(personal))
	}
	if (synctrust.Scope{Vault: team}).Allows(synctrust.TypeContact) {
		t.Fatal("a team vault allows contacts; whom a person verified is theirs alone")
	}
	dst := domain.NewVaultData()
	if err := synctrust.Apply(dst, []synctrust.Entry{e}); err != nil {
		t.Fatal(err)
	}
	if dst.Sync == nil || len(dst.Sync.Contacts) != 1 || dst.Sync.Contacts[0].IdentityDigest != digest || !dst.Sync.Contacts[0].Verified {
		t.Fatalf("contacts after apply = %+v", dst.Sync)
	}
	bad := synctrust.Entry{Type: synctrust.TypeContact, Key: "zz", Data: []byte(`{"account":"zz","identityDigest":"00","verified":false}`)}
	if err := synctrust.Validate(bad); !errors.Is(err, synctrust.ErrRecordData) {
		t.Fatalf("err = %v, want ErrRecordData for a contact with a malformed account or digest", err)
	}
}

func TestBindingsSyncInThePersonalVaultOnly(t *testing.T) {
	d := mixedVault()
	b := domain.CredentialBinding{VaultID: team, ConnectionID: "tc", UserID: "u1", IdentityID: "k1"}
	d.Sync = &domain.VaultSync{Bindings: []domain.CredentialBinding{b}}
	e, ok := scoped(t, d, synctrust.Scope{})["credential-binding "+b.Key()]
	if !ok {
		t.Fatal("a binding follows its member to every device of theirs")
	}
	if (synctrust.Scope{Vault: team}).Allows(synctrust.TypeCredentialBinding) {
		t.Fatal("a team vault allows bindings; which credential a member uses is theirs alone")
	}
	dst := domain.NewVaultData()
	if err := synctrust.Apply(dst, []synctrust.Entry{e}); err != nil || len(dst.Sync.Bindings) != 1 {
		t.Fatalf("apply: %v", err)
	}
	both := synctrust.Entry{Type: synctrust.TypeCredentialBinding, Key: "t/c/u",
		Data: []byte(`{"vaultId":"t","connectionId":"c","userId":"u","identityId":"k","passwordId":"p"}`)}
	if err := synctrust.Validate(both); !errors.Is(err, synctrust.ErrRecordData) {
		t.Fatalf("err = %v, want ErrRecordData: a binding names exactly one credential", err)
	}
}
