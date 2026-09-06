package plugin

import (
	"errors"
	"testing"
)

// Replication carries the contents of a scope, so a transport without one has nothing to carry.
// Saying so at install is kinder than a plugin that starts and then quietly does nothing.
func TestReplicationWithoutAScopeIsRefused(t *testing.T) {
	m := &Manifest{
		ID:           "com.example.sync",
		Capabilities: CapabilitySet{Replica: &ReplicaCaps{}},
	}

	if err := m.ValidateCapabilities(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("err = %v, want ErrInvalidManifest", err)
	}
}

// The pair a synchronisation plugin actually declares.
func TestReplicationWithAScopeIsAccepted(t *testing.T) {
	m := &Manifest{
		ID: "com.example.sync",
		Capabilities: CapabilitySet{
			Scope:   &ScopeCaps{},
			Replica: &ReplicaCaps{},
		},
	}

	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("a scope with a transport was refused: %v", err)
	}
	if !m.Replicates() || !m.NeedsScope() {
		t.Fatal("the manifest does not report the capabilities it declares")
	}
}

// A scope on its own is fine: a plugin may read what the user exposes to it without carrying any of
// it anywhere, which is what a provisioning plugin does.
func TestAScopeWithoutReplicationIsFine(t *testing.T) {
	m := &Manifest{ID: "com.example.sync", Capabilities: CapabilitySet{Scope: &ScopeCaps{}}}

	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("a scope without a transport was refused: %v", err)
	}
	if m.Replicates() {
		t.Error("a manifest with no replica capability reports that it replicates")
	}
}

// Carrying a scope off the machine is counted, so a version that starts doing it has to reach the
// user rather than arriving with an update.
func TestReplicationIsAPermission(t *testing.T) {
	before := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: CapabilitySet{Scope: &ScopeCaps{}},
	})
	after := PermissionSetFromManifest(&Manifest{
		ID:           "com.example.sync",
		Capabilities: CapabilitySet{Scope: &ScopeCaps{}, Replica: &ReplicaCaps{}},
	})

	added, widened := after.Widens(before)
	if !widened || len(added) != 1 || added[0] != PermissionReplica {
		t.Fatalf("added = %v, widened = %v; want exactly [%s]", added, widened, PermissionReplica)
	}
}
