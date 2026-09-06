package domain

import (
	"errors"
	"fmt"
)

// ErrProvenanceViolation indicates an object composed in a way its owner is not permitted to
// compose - a plugin's object reaching for material that belongs to the user.
var ErrProvenanceViolation = errors.New("object references material its owner may not use")

// ValidateProvenance enforces the composition rule of ADR-022 for one connection.
//
// A connection a plugin provisioned may not reference anything stored in the vault: no identity, no
// password, neither in its users nor in its jump hops. The rule is about composition rather than
// access, and that distinction is the point. Stopping a plugin from *reading* a key is not enough
// while it can hand the core a connection that uses one - the key never leaves the vault, the plugin
// never sees it, and it is spent against a host the plugin chose. That is the same outcome as theft
// by a longer route, and it closes with the reference rather than with the read.
//
// What such a connection is meant to do instead is authenticate through its own plugin, so the key
// stays with the plugin and the core sees only a signature, or carry no stored credential at all and
// take one from the plugin when the session opens.
//
// A core-owned connection is unaffected: the user's own connections use the user's own vault, which
// is the ordinary case and the one that must stay free.
func (c *Connection) ValidateProvenance() error {
	if c.Owner.IsCore() {
		return nil
	}
	pluginID, _ := c.Owner.PluginID()
	for i := range c.Users {
		if err := refuseVaultSecrets(c.Users[i].KeyAuth, c.Users[i].PassAuth, pluginID, "user "+c.Users[i].ID); err != nil {
			return err
		}
	}
	for i := range c.JumpChain.Hops {
		hop := &c.JumpChain.Hops[i]
		if err := refuseVaultSecrets(hop.KeyAuth, hop.PassAuth, pluginID, "jump hop "+hop.ID); err != nil {
			return err
		}
	}
	return nil
}

// refuseVaultSecrets reports a reference to vault-stored material.
//
// An empty configuration references nothing and is allowed: a half-filled connection is a draft, and
// refusing one would reject an ordinary editing state rather than an attack.
func refuseVaultSecrets(key *KeyAuthConfig, pass *PasswordAuthConfig, pluginID, where string) error {
	if key != nil && len(key.IdentityIDs) > 0 {
		return fmt.Errorf("%w: %s of a connection owned by plugin %s references a vault key",
			ErrProvenanceViolation, where, pluginID)
	}
	if pass != nil && pass.VaultRef != "" {
		return fmt.Errorf("%w: %s of a connection owned by plugin %s references a vault password",
			ErrProvenanceViolation, where, pluginID)
	}
	return nil
}
