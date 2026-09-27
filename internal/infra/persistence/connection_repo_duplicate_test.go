package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain"
)

func ruleIDs() func() (string, error) {
	n := 0
	return func() (string, error) {
		n++
		return fmt.Sprintf("r%d", n), nil
	}
}

func duplicateFixture() *memVault {
	d := domain.NewVaultData()
	d.Connections = []domain.Connection{
		{ID: "a", Name: "a", FolderID: "f", Host: "h", Port: 22, Order: 0},
		{ID: "src", Name: "vnc box", FolderID: "f", Host: "h", Port: 5900, Order: 1, Protocol: "vnc",
			PluginFields: map[string]string{"password": domain.PluginSecretRef("src", "password")}},
		{ID: "b", Name: "b", FolderID: "f", Host: "h", Port: 22, Order: 2},
		{ID: "other", Name: "other", FolderID: "g", Host: "h", Port: 22, Order: 2},
	}
	d.PluginSecrets = map[string][]byte{domain.PluginSecretRef("src", "password"): []byte("s3cret")}
	return &memVault{data: d}
}

func orderOf(t *testing.T, data *domain.VaultData, id string) int {
	t.Helper()
	for _, c := range data.Connections {
		if c.ID == id {
			return c.Order
		}
	}
	t.Fatalf("connection %s not found", id)
	return 0
}

func TestDuplicateLandsDirectlyAfterTheOriginal(t *testing.T) {
	v := duplicateFixture()
	created, err := NewConnectionRepo(v).Duplicate(context.Background(), "src", "vnc box - copy", ruleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if created.ID == "" || created.ID == "src" || created.Name != "vnc box - copy" {
		t.Fatalf("created = %+v, want a new id and the given name", created)
	}
	if got := orderOf(t, v.data, created.ID); got != 2 {
		t.Errorf("copy order = %d, want 2, right after the source at 1", got)
	}
	if got := orderOf(t, v.data, "src"); got != 1 {
		t.Errorf("source order = %d, want it left at 1; moving it too would tie it with its copy", got)
	}
	if got := orderOf(t, v.data, "b"); got != 3 {
		t.Errorf("following sibling order = %d, want it moved down to 3", got)
	}
	if got := orderOf(t, v.data, "a"); got != 0 {
		t.Errorf("preceding sibling order = %d, want it left at 0", got)
	}
	if got := orderOf(t, v.data, "other"); got != 2 {
		t.Errorf("connection in another folder moved to %d; only the source's folder makes room", got)
	}
}

func TestDuplicateCopiesTheSecretValue(t *testing.T) {
	v := duplicateFixture()
	created, err := NewConnectionRepo(v).Duplicate(context.Background(), "src", "copy", ruleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	ref := created.PluginFields["password"]
	if ref == domain.PluginSecretRef("src", "password") {
		t.Fatal("the copy points at the source's secret; clearing it on the copy would delete the original's")
	}
	if got := string(v.data.PluginSecrets[ref]); got != "s3cret" {
		t.Errorf("copied secret = %q, want the source's value", got)
	}
	v.data.PluginSecrets[ref][0] = 'X'
	if string(v.data.PluginSecrets[domain.PluginSecretRef("src", "password")]) != "s3cret" {
		t.Error("the copy's secret shares bytes with the source's")
	}
}

// UpdateData keeps whatever a failed mutation changed, so a refused duplicate must have changed
// nothing at all.
func TestDuplicateWithAMissingSecretChangesNothing(t *testing.T) {
	v := duplicateFixture()
	delete(v.data.PluginSecrets, domain.PluginSecretRef("src", "password"))
	before := len(v.data.Connections)

	_, err := NewConnectionRepo(v).Duplicate(context.Background(), "src", "copy", ruleIDs())
	if !errors.Is(err, domain.ErrInvalidConnectionConfig) {
		t.Fatalf("err = %v, want ErrInvalidConnectionConfig", err)
	}
	if len(v.data.Connections) != before || orderOf(t, v.data, "b") != 2 || len(v.data.PluginSecrets) != 0 {
		t.Errorf("a refused duplicate left changes behind: %+v / %v", v.data.Connections, v.data.PluginSecrets)
	}
}

func TestDuplicateOfAnUnknownConnectionIsNotFound(t *testing.T) {
	_, err := NewConnectionRepo(duplicateFixture()).Duplicate(context.Background(), "nope", "copy", ruleIDs())
	if !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Errorf("err = %v, want ErrConnectionNotFound", err)
	}
}
