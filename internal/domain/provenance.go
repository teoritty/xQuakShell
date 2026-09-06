package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidOwner indicates an owner that names no one, or a stored owner nothing can interpret.
var ErrInvalidOwner = errors.New("invalid object owner")

// ownerCore and ownerPlugin are the two authorities that can create an object. The zero value is
// deliberately the core: see Owner.
type ownerKind int

const (
	ownerCore ownerKind = iota
	ownerPlugin
)

// storedCoreOwner and storedPluginPrefix are the on-disk forms. They are written into every
// connection, folder and identity in the vault, so changing one migrates nothing - it stops
// matching what is recorded, and a test pins both.
const (
	storedCoreOwner    = "core"
	storedPluginPrefix = "plugin:"
)

// Owner records which authority created an object and owns its lifecycle (ADR-022).
//
// The fields are unexported and there is no setter. That is the whole design: an owner set at
// creation and never reassigned is what stops a plugin claiming an object the user made, and with
// an exported field that would be a convention rather than a fact. A test asserts the type has no
// exported field, because the guarantee is structural and a future edit could give it one.
//
// The zero value is the core, which is what lets a vault written before this axis existed be read
// without a migration: every object in it has no owner field, so every object decodes as the user's
// own. Any other zero value would silently reassign everything they ever created.
type Owner struct {
	kind     ownerKind
	pluginID string
}

// CoreOwner returns the owner of everything the user creates.
func CoreOwner() Owner {
	return Owner{kind: ownerCore}
}

// PluginOwner returns the owner for objects a plugin provisions.
//
// An empty id is refused rather than accepted as some anonymous owner: a second nameless authority
// beside the core would make every "is this mine" check guess which one it was looking at.
func PluginOwner(pluginID string) (Owner, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return Owner{}, fmt.Errorf("%w: a plugin owner needs a plugin id", ErrInvalidOwner)
	}
	return Owner{kind: ownerPlugin, pluginID: pluginID}, nil
}

// IsCore reports whether the object belongs to the user rather than to a plugin.
func (o Owner) IsCore() bool {
	return o.kind == ownerCore
}

// PluginID returns the owning plugin, and false for a core-owned object.
func (o Owner) PluginID() (string, bool) {
	if o.kind != ownerPlugin {
		return "", false
	}
	return o.pluginID, true
}

// String names the owner for an audit line or an error message.
func (o Owner) String() string {
	if o.kind == ownerPlugin {
		return storedPluginPrefix + o.pluginID
	}
	return storedCoreOwner
}

// MarshalJSON writes the stored form, which is the same string String renders.
func (o Owner) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

// UnmarshalJSON reads the stored form.
//
// An absent field never reaches here at all and keeps the zero value, which is the core. A null or
// an empty string does reach here, from a build that had the field and nothing to put in it, and
// gets the same answer for the same reason.
//
// null needs no branch of its own: encoding/json leaves a string untouched when it decodes null, so
// it arrives as the empty string the case below already answers. A branch for it read as a separate
// decision and no test could tell it from its absence.
//
// Anything else is an error rather than a guess. Guessing "core" would hand a plugin's object to the
// user and guessing "plugin" would need an id there is none of, so either guess turns an unreadable
// vault into a readable one saying something different.
func (o *Owner) UnmarshalJSON(raw []byte) error {
	var stored string
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidOwner, raw)
	}
	switch {
	case stored == "" || stored == storedCoreOwner:
		*o = CoreOwner()
		return nil
	case strings.HasPrefix(stored, storedPluginPrefix):
		owner, err := PluginOwner(strings.TrimPrefix(stored, storedPluginPrefix))
		if err != nil {
			return err
		}
		*o = owner
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidOwner, stored)
	}
}
