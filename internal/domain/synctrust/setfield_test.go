package synctrust_test

import (
	"encoding/json"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

func TestSetFieldWritesOneFieldThroughTheValidators(t *testing.T) {
	d := populatedVault()
	if err := synctrust.SetField(d, synctrust.TypeConnection, "c1", []string{"host"}, json.RawMessage(`"mine.example"`)); err != nil {
		t.Fatal(err)
	}
	if d.Connections[0].Host != "mine.example" || d.Connections[0].Name != "web" {
		t.Fatalf("connection = %+v; only the one field may change", d.Connections[0])
	}
	if err := synctrust.SetField(d, synctrust.TypeConnection, "c1", []string{"port"}, json.RawMessage(`70000`)); !errors.Is(err, synctrust.ErrRecordData) {
		t.Fatalf("an invalid value: %v, want ErrRecordData from the domain validator", err)
	}
	if d.Connections[0].Port != 22 {
		t.Fatal("a refused value was written")
	}
}

func TestSetFieldInsideAnIDArray(t *testing.T) {
	d := domain.NewVaultData()
	d.Connections = []domain.Connection{{ID: "c1", Port: 22, Users: []domain.ConnectionUser{{ID: "u1", Username: "a"}, {ID: "u2", Username: "b"}}}}
	if err := synctrust.SetField(d, synctrust.TypeConnection, "c1", []string{"users", "[id=u2]", "username"}, json.RawMessage(`"root"`)); err != nil {
		t.Fatal(err)
	}
	if d.Connections[0].Users[1].Username != "root" || d.Connections[0].Users[0].Username != "a" {
		t.Fatalf("users = %+v", d.Connections[0].Users)
	}
}

func TestSetFieldRefusals(t *testing.T) {
	d := populatedVault()
	for name, c := range map[string]struct {
		ty   synctrust.RecordType
		key  string
		path []string
	}{
		"unknown item":       {synctrust.TypeConnection, "nope", []string{"host"}},
		"empty path":         {synctrust.TypeConnection, "c1", nil},
		"missing array item": {synctrust.TypeConnection, "c1", []string{"users", "[id=zz]", "username"}},
		"the item's own key": {synctrust.TypeConnection, "c1", []string{"id"}},
		"through a scalar":   {synctrust.TypeConnection, "c1", []string{"host", "x"}},
	} {
		if err := synctrust.SetField(d, c.ty, c.key, c.path, json.RawMessage(`"x"`)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
