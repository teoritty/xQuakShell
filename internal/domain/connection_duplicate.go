package domain

import (
	"fmt"
	"strings"
)

// DuplicatedConnection is a copy ready to be stored, and the secrets that must be copied with it.
type DuplicatedConnection struct {
	Connection Connection
	// SecretCopies maps each secret reference the copy uses to the source reference whose value
	// it must receive.
	SecretCopies map[string]string
}

// DuplicateConnection builds an independent copy of src under newID and the given name.
//
// Independent means nothing the copy owns is shared with the source. Its secret plugin fields get
// references of their own, and its forward rules get new ids: rule ids are checked for uniqueness
// across every connection and key live listeners at runtime, so a copy that kept them would fail
// its first save and collide with the original's forwards the first time both were connected.
//
// What the copy does share is what the vault keeps as a library - saved passwords and keys are
// referenced by id, exactly as two connections set up by hand to use the same key would.
func DuplicateConnection(src Connection, newID, name string, newRuleID func() (string, error)) (DuplicatedConnection, error) {
	name = strings.TrimSpace(name)
	if newID == "" || name == "" {
		return DuplicatedConnection{}, fmt.Errorf("duplicate connection %s: id and name are required: %w", src.ID, ErrInvalidConnectionConfig)
	}
	out := CloneConnection(src)
	out.ID = newID
	out.Name = name

	rules, err := renumberForwardRules(src.ForwardRules, newRuleID)
	if err != nil {
		return DuplicatedConnection{}, fmt.Errorf("duplicate connection %s: %w", src.ID, err)
	}
	out.ForwardRules = rules

	copies := make(map[string]string)
	for fieldID, value := range out.PluginFields {
		if value != PluginSecretRef(src.ID, fieldID) {
			continue
		}
		ref := PluginSecretRef(newID, fieldID)
		out.PluginFields[fieldID] = ref
		copies[ref] = value
	}

	if err := out.Validate(); err != nil {
		return DuplicatedConnection{}, fmt.Errorf("duplicate connection %s: %w", src.ID, err)
	}
	return DuplicatedConnection{Connection: out, SecretCopies: copies}, nil
}

func renumberForwardRules(in []ForwardRule, newRuleID func() (string, error)) ([]ForwardRule, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]ForwardRule, len(in))
	for i, rule := range in {
		id, err := newRuleID()
		if err != nil {
			return nil, err
		}
		rule.ID = id
		out[i] = rule
	}
	return out, nil
}
