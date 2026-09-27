package domain

import (
	"errors"
	"fmt"
	"testing"
)

func sequentialRuleIDs() func() (string, error) {
	n := 0
	return func() (string, error) {
		n++
		return fmt.Sprintf("rule-new-%d", n), nil
	}
}

func sourceConnection() Connection {
	return Connection{
		ID:       "src",
		FolderID: "f1",
		Name:     "prod-db",
		Host:     "10.0.0.5",
		Port:     22,
		Order:    3,
		Tags:     []string{"prod"},
		Users: []ConnectionUser{
			{ID: "u1", Username: "root", Auth: "password", PassAuth: &PasswordAuthConfig{VaultRef: "pw-1"}},
		},
		DefaultUserID: "u1",
		ForwardRules: []ForwardRule{
			{ID: "rule-a", Kind: "local", BindPort: 8080, TargetHost: "localhost", TargetPort: 80},
			{ID: "rule-b", Kind: "dynamic", BindPort: 1080},
		},
		Protocol: "vnc",
		PluginFields: map[string]string{
			"password": PluginSecretRef("src", "password"),
			"quality":  "high",
			"foreign":  PluginSecretRef("other", "foreign"),
		},
	}
}

func TestDuplicateCarriesEverythingUnderANewIdentity(t *testing.T) {
	src := sourceConnection()
	dup, err := DuplicateConnection(src, "copy", "prod-db - copy", sequentialRuleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	c := dup.Connection
	if c.ID != "copy" || c.Name != "prod-db - copy" {
		t.Errorf("id/name = %q/%q, want copy / prod-db - copy", c.ID, c.Name)
	}
	if c.Host != src.Host || c.Port != src.Port || c.FolderID != src.FolderID || c.Protocol != src.Protocol {
		t.Errorf("copy lost its target: %+v", c)
	}
	if len(c.Users) != 1 || c.Users[0].PassAuth.VaultRef != "pw-1" || c.DefaultUserID != "u1" {
		t.Errorf("users = %+v default %q; the saved password is a vault library entry and is shared", c.Users, c.DefaultUserID)
	}
}

// The copy must be its own record: editing it cannot reach back into the source.
func TestDuplicateSharesNoMutableState(t *testing.T) {
	src := sourceConnection()
	dup, err := DuplicateConnection(src, "copy", "x", sequentialRuleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	dup.Connection.Tags[0] = "changed"
	dup.Connection.Users[0].Username = "changed"
	dup.Connection.ForwardRules[0].BindPort = 1
	dup.Connection.PluginFields["quality"] = "changed"
	if src.Tags[0] != "prod" || src.Users[0].Username != "root" || src.ForwardRules[0].BindPort != 8080 || src.PluginFields["quality"] != "high" {
		t.Errorf("editing the copy changed the source: %+v", src)
	}
}

// Rule ids are unique across every connection and key live listeners, so the copy's are new.
func TestDuplicateRenumbersForwardRules(t *testing.T) {
	dup, err := DuplicateConnection(sourceConnection(), "copy", "x", sequentialRuleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	rules := dup.Connection.ForwardRules
	if len(rules) != 2 || rules[0].ID != "rule-new-1" || rules[1].ID != "rule-new-2" {
		t.Fatalf("rule ids = %+v, want freshly generated ones", rules)
	}
	if rules[0].BindPort != 8080 || rules[1].Kind != "dynamic" {
		t.Errorf("a renumbered rule lost its definition: %+v", rules)
	}
}

// A secret field of the source moves to a reference of the copy's own; anything else is copied
// as it is, including a reference that is not the source's.
func TestDuplicateGivesTheCopyItsOwnSecrets(t *testing.T) {
	dup, err := DuplicateConnection(sourceConnection(), "copy", "x", sequentialRuleIDs())
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	fields := dup.Connection.PluginFields
	if want := PluginSecretRef("copy", "password"); fields["password"] != want {
		t.Errorf("secret field = %q, want %q", fields["password"], want)
	}
	if dup.SecretCopies[PluginSecretRef("copy", "password")] != PluginSecretRef("src", "password") {
		t.Errorf("secret copies = %v, want the copy's ref to take the source's value", dup.SecretCopies)
	}
	if fields["quality"] != "high" || fields["foreign"] != PluginSecretRef("other", "foreign") {
		t.Errorf("non-secret fields changed: %v", fields)
	}
	if len(dup.SecretCopies) != 1 {
		t.Errorf("secret copies = %v, want exactly the one the source owns", dup.SecretCopies)
	}
}

func TestDuplicateRefusesAnEmptyName(t *testing.T) {
	for _, name := range []string{"", "   "} {
		_, err := DuplicateConnection(sourceConnection(), "copy", name, sequentialRuleIDs())
		if !errors.Is(err, ErrInvalidConnectionConfig) {
			t.Errorf("name %q: err = %v, want ErrInvalidConnectionConfig", name, err)
		}
	}
}

func TestDuplicateReportsARuleIDFailure(t *testing.T) {
	boom := errors.New("no entropy")
	_, err := DuplicateConnection(sourceConnection(), "copy", "x", func() (string, error) { return "", boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the generator's error", err)
	}
}

func TestDuplicateValidatesTheCopy(t *testing.T) {
	src := sourceConnection()
	src.Port = 70000
	if _, err := DuplicateConnection(src, "copy", "x", sequentialRuleIDs()); !errors.Is(err, ErrInvalidConnectionConfig) {
		t.Errorf("err = %v, want an invalid source to stay refused in its copy", err)
	}
}
