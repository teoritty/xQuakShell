package plugin

import "fmt"

// ReplicaCaps declares that a plugin carries the contents of its scope to the user's other devices
// (ADR-022, port A).
//
// It carries no fields for the same reason ScopeCaps does not: what travels, when, and in what
// order are the core's decisions, and the plugin's job is to move bytes it cannot read. Anything a
// plugin could declare here would be a lever on data it is not supposed to understand.
type ReplicaCaps struct{}

// Replicates reports whether the manifest declares the replication transport.
func (m *Manifest) Replicates() bool {
	return m != nil && m.Capabilities.Replica != nil
}

// validateReplicaCaps refuses a transport with nothing to carry.
//
// Replication moves the contents of a scope, so a plugin that declares one without the other has
// either forgotten half its manifest or is asking for a verb it can never usefully be called with.
// Saying so at install is kinder than a plugin that starts and then does nothing.
func (m *Manifest) validateReplicaCaps() error {
	if m.Capabilities.Replica == nil {
		return nil
	}
	if m.Capabilities.Scope == nil {
		return fmt.Errorf("%w: capabilities.replica needs capabilities.scope, or there is nothing to replicate", ErrInvalidManifest)
	}
	return nil
}
