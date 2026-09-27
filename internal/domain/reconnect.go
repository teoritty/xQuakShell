package domain

// ReconnectSettings controls what happens after an established session loses its connection.
//
// Both fields are pointers for the reason UpdateSettings.CheckOnStartup is: they were added after
// vaults existed and both default to on, so a plain bool would decode as false in every older vault
// and switch the feature off for exactly the users who never chose to. nil means "never
// configured" and resolves to on; false is a deliberate opt-out.
type ReconnectSettings struct {
	// AutoReconnect retries a lost session on a growing delay until it connects or the user stops it.
	AutoReconnect *bool `json:"autoReconnect,omitempty"`
	// PreserveContext carries the lost session's terminal - scrollback and screen - over to the
	// session that replaces it, instead of starting from an empty terminal.
	PreserveContext *bool `json:"preserveContext,omitempty"`
}

// AutoReconnectEnabled resolves the tri-state to the effective behaviour.
func (r ReconnectSettings) AutoReconnectEnabled() bool {
	return r.AutoReconnect == nil || *r.AutoReconnect
}

// PreserveContextEnabled resolves the tri-state to the effective behaviour.
func (r ReconnectSettings) PreserveContextEnabled() bool {
	return r.PreserveContext == nil || *r.PreserveContext
}
