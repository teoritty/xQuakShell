package synctrust

import (
	"encoding/json"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDescribeNamesEachItemTheWayThePersonNamedIt(t *testing.T) {
	for _, tc := range []struct {
		t    RecordType
		data any
		want string
	}{
		{TypeConnection, domain.Connection{ID: "0922d087", Name: "prod-web", Host: "10.0.0.1"}, "prod-web"},
		{TypeFolder, domain.ConnectionFolder{ID: "f1", Name: "Prod"}, "Prod"},
		{TypeIdentity, identityData{Identity: domain.SSHIdentity{ID: "k1", Comment: "deploy key"}}, "deploy key"},
		{TypePassword, passwordData{ID: "p1", Label: "db root", Value: []byte("secret")}, "db root"},
		{TypeKnownHost, knownHostData{Line: "git.example ssh-ed25519 AAAA"}, "git.example"},
		{TypeKnownHost, knownHostData{Line: "@cert-authority *.example ssh-ed25519 AAAA"}, "*.example"},
		{TypePeerTrust, domain.PeerTrustEntry{Scope: "p", Subject: "host-a"}, "host-a"},
		{SettingsType("lockout"), map[string]any{"enabled": true}, ""},
	} {
		if got := Describe(tc.t, mustJSON(t, tc.data)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.t, got, tc.want)
		}
	}
	if got := Describe(TypePassword, mustJSON(t, passwordData{ID: "p1", Label: "", Value: []byte("hunter2")})); got != "" {
		t.Fatalf("a password without a label is described as %q; the value must never stand in for a name", got)
	}
	if got := Describe(TypeConnection, []byte("not json")); got != "" {
		t.Fatalf("undecodable data is described as %q", got)
	}
}

func TestInternalRecordsAreTheEnginesOwn(t *testing.T) {
	for _, rt := range []RecordType{TypeVaultMeta, TypeConflict, TypeContact, TypeCredentialBinding} {
		if !Internal(rt) {
			t.Errorf("%s is shown to the person; nothing they made or can act on", rt)
		}
	}
	for _, rt := range []RecordType{TypeConnection, TypeFolder, TypeIdentity, TypePassword, TypeKnownHost, SettingsType("plugins")} {
		if Internal(rt) {
			t.Errorf("%s is hidden; the person created it and can restore it", rt)
		}
	}
}

func TestDescribePathResolvesSelectorsToNames(t *testing.T) {
	d := &domain.VaultData{Connections: []domain.Connection{{ID: "c1", Name: "web", Host: "h", Port: 22,
		Users: []domain.ConnectionUser{{ID: "u-1", Username: "deploy"}}}}}
	got := DescribePath(d, TypeConnection, "c1", []string{"users", "[id=u-1]", "passAuth", "passwordId"})
	if !slices.Equal(got, []string{"", "deploy", "", ""}) {
		t.Fatalf("path names = %q; the user is named by its login, plain fields are left to translate", got)
	}
	if got := DescribePath(d, TypeConnection, "c1", []string{"users", "[id=gone]", "username"}); !slices.Equal(got, []string{"", "", ""}) {
		t.Fatalf("path names = %q; a vanished element has no name", got)
	}
	if got := DescribeItem(d, TypeConnection, "c1"); got != "web" {
		t.Fatalf("item = %q", got)
	}
	if got := DescribeItem(d, TypeConnection, "nope"); got != "" {
		t.Fatalf("a missing item is described as %q", got)
	}
}
