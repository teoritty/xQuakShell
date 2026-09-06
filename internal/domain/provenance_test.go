package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"xquakshell/internal/domain"
)

// The zero value is the core, and that is what makes a vault written before provenance existed
// readable without a migration: every object in it has no owner field, so every object decodes as
// the user's own. Any other zero value would silently reassign everything they ever created.
func TestTheZeroOwnerIsTheCore(t *testing.T) {
	var owner domain.Owner

	if !owner.IsCore() {
		t.Fatal("the zero owner is not the core")
	}
	if id, ok := owner.PluginID(); ok {
		t.Fatalf("the zero owner names plugin %q", id)
	}
}

// A plugin owner carries the plugin it belongs to, and the core owner carries nothing to carry.
func TestAPluginOwnerNamesItsPlugin(t *testing.T) {
	owner, err := domain.PluginOwner("com.example.sync")
	if err != nil {
		t.Fatalf("PluginOwner err = %v, want nil", err)
	}

	if owner.IsCore() {
		t.Fatal("a plugin owner reports as the core")
	}
	id, ok := owner.PluginID()
	if !ok || id != "com.example.sync" {
		t.Fatalf("PluginID = %q, %v; want the plugin id", id, ok)
	}
}

// An owner with no plugin id belongs to no plugin. Allowing one would create a second anonymous
// owner alongside the core, and every check asking "is this mine" would have to guess which.
func TestAPluginOwnerNeedsAPluginID(t *testing.T) {
	for _, id := range []string{"", "   "} {
		if _, err := domain.PluginOwner(id); !errors.Is(err, domain.ErrInvalidOwner) {
			t.Errorf("PluginOwner(%q) err = %v, want ErrInvalidOwner", id, err)
		}
	}
}

// The structural guarantee behind the whole axis: an owner cannot be reassigned, because there is
// nothing to assign to. A single exported field would make "owner is set at creation" a convention
// rather than a fact, and the escalation this axis exists to prevent is a plugin claiming an object
// it did not create.
func TestOwnerHasNoExportedFieldToAssignTo(t *testing.T) {
	ownerType := reflect.TypeOf(domain.Owner{})

	for i := range ownerType.NumField() {
		if field := ownerType.Field(i); field.IsExported() {
			t.Errorf("Owner.%s is exported, so an owner can be reassigned by writing to it", field.Name)
		}
	}
}

// The stored form is an on-disk contract: it is written into every connection, folder and identity
// in the vault. Changing one of these strings does not migrate anything - it stops matching what is
// recorded, and every object written before the change decodes as something else or fails to decode.
func TestTheStoredFormIsAnOnDiskContract(t *testing.T) {
	core, err := json.Marshal(domain.CoreOwner())
	if err != nil {
		t.Fatalf("marshal core: %v", err)
	}
	if string(core) != `"core"` {
		t.Errorf("core owner marshalled as %s, want \"core\"", core)
	}

	owner, _ := domain.PluginOwner("com.example.sync")
	plugin, err := json.Marshal(owner)
	if err != nil {
		t.Fatalf("marshal plugin: %v", err)
	}
	if string(plugin) != `"plugin:com.example.sync"` {
		t.Errorf("plugin owner marshalled as %s, want \"plugin:com.example.sync\"", plugin)
	}
}

// Both forms have to survive the trip to the vault and back, or ownership changes every time the
// vault is read - which is the one thing an immutable axis must never do.
func TestAnOwnerSurvivesAJSONRoundTrip(t *testing.T) {
	pluginOwner, _ := domain.PluginOwner("com.example.sync")

	for _, want := range []domain.Owner{domain.CoreOwner(), pluginOwner} {
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %v: %v", want, err)
		}
		var got domain.Owner
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if got != want {
			t.Errorf("round trip turned %v into %v", want, got)
		}
	}
}

type ownedThing struct {
	Name  string       `json:"name"`
	Owner domain.Owner `json:"owner"`
}

// An object stored before this axis existed has no owner field at all. It must decode as the user's
// own rather than as an error, because that is every object in every existing vault.
func TestAnAbsentOwnerFieldDecodesAsTheCore(t *testing.T) {
	var thing ownedThing

	if err := json.Unmarshal([]byte(`{"name":"prod"}`), &thing); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !thing.Owner.IsCore() {
		t.Fatal("an object with no owner field did not decode as core-owned")
	}
}

// A null, or an empty string, reaches this the same way an absent field does - from a vault written
// by a build that had the field but nothing to put in it. Same answer, for the same reason.
func TestANullOrEmptyOwnerDecodesAsTheCore(t *testing.T) {
	for _, raw := range []string{`{"name":"prod","owner":null}`, `{"name":"prod","owner":""}`} {
		var thing ownedThing
		if err := json.Unmarshal([]byte(raw), &thing); err != nil {
			t.Errorf("unmarshal %s: %v", raw, err)
			continue
		}
		if !thing.Owner.IsCore() {
			t.Errorf("%s did not decode as core-owned", raw)
		}
	}
}

// A stored owner nobody can interpret must be an error rather than a guess. Guessing "core" would
// hand a plugin's object to the user, and guessing "plugin" would need a plugin id there is none of;
// either way an unreadable vault would quietly become a readable one saying something else.
func TestAnUnreadableOwnerIsRefused(t *testing.T) {
	for _, raw := range []string{`"nonsense"`, `"plugin:"`, `"plugin"`, `42`, `{"kind":"core"}`} {
		var owner domain.Owner
		if err := json.Unmarshal([]byte(raw), &owner); err == nil {
			t.Errorf("%s decoded as %v, want an error", raw, owner)
		}
	}
}

// Owners are compared all over the composition rules, so they must compare by value. A type holding
// a slice or a map would not, and == would stop compiling rather than stop working - but only once
// someone tried.
func TestOwnersCompareByValue(t *testing.T) {
	first, _ := domain.PluginOwner("com.example.sync")
	second, _ := domain.PluginOwner("com.example.sync")
	other, _ := domain.PluginOwner("com.example.other")

	if first != second {
		t.Error("two owners of the same plugin are not equal")
	}
	if first == other {
		t.Error("owners of different plugins are equal")
	}
	if first == domain.CoreOwner() {
		t.Error("a plugin owner equals the core owner")
	}
}

// String is what an audit line and an error message render, so it has to name the plugin rather
// than print a struct nobody can read.
func TestStringNamesTheOwner(t *testing.T) {
	owner, _ := domain.PluginOwner("com.example.sync")

	if got := domain.CoreOwner().String(); got != "core" {
		t.Errorf("core String() = %q, want \"core\"", got)
	}
	if got := owner.String(); got != "plugin:com.example.sync" {
		t.Errorf("plugin String() = %q, want \"plugin:com.example.sync\"", got)
	}
}
