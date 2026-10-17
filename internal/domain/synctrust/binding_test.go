package synctrust_test

import (
	"encoding/json"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

func connData(c domain.Connection) []byte { b, _ := json.Marshal(c); return b }

func TestAddressChangedSeesHostPortAndJumps(t *testing.T) {
	base := domain.Connection{ID: "c", Name: "db", Host: "db.example", Port: 22}
	for name, edit := range map[string]func(*domain.Connection){
		"host": func(c *domain.Connection) { c.Host = "evil.example" },
		"port": func(c *domain.Connection) { c.Port = 2222 },
		"jump": func(c *domain.Connection) { c.JumpChain.Hops = []domain.JumpHop{{ID: "h", Host: "bastion"}} },
	} {
		changed := base
		edit(&changed)
		if !synctrust.AddressChanged(connData(base), connData(changed)) {
			t.Errorf("a changed %s is not an address change", name)
		}
	}
	emptyHops := base
	emptyHops.JumpChain.Hops = []domain.JumpHop{}
	if synctrust.AddressChanged(connData(base), connData(emptyHops)) {
		t.Error("an empty jump chain encoded differently is an address change")
	}
	renamed := base
	renamed.Name, renamed.Tags = "database", []string{"x"}
	if synctrust.AddressChanged(connData(base), connData(renamed)) {
		t.Error("a rename is an address change")
	}
}

func TestSuspendBindingsOfOneConnection(t *testing.T) {
	d := domain.NewVaultData()
	d.Sync = &domain.VaultSync{Bindings: []domain.CredentialBinding{
		{VaultID: "t", ConnectionID: "c", UserID: "u1", IdentityID: "k"},
		{VaultID: "t", ConnectionID: "other", UserID: "u1", IdentityID: "k"},
		{VaultID: "t2", ConnectionID: "c", UserID: "u1", IdentityID: "k"},
	}}
	synctrust.SuspendBindings(d, "t", "c", "carol")
	if d.Sync.Bindings[0].SuspendedBy != "carol" || d.Sync.Bindings[1].SuspendedBy != "" || d.Sync.Bindings[2].SuspendedBy != "" {
		t.Fatalf("bindings = %+v; only that vault's connection's bindings suspend", d.Sync.Bindings)
	}
}
