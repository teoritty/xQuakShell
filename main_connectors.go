package main

import "ssh-client/internal/domain"

// newSessionConnectors returns non-SSH session protocol connectors (composition root only).
// Built-in connectors were removed; plugins register here later.
func newSessionConnectors() []domain.SessionConnector {
	return nil
}
