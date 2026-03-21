package main

import (
	"ssh-client/internal/domain"
	"ssh-client/internal/infra/connectors"
)

// newSessionConnectors returns all non-SSH session protocol connectors (composition root only).
func newSessionConnectors() []domain.SessionConnector {
	return []domain.SessionConnector{
		connectors.NewTelnetConnector(),
		connectors.NewSerialConnector(),
		connectors.NewRDPConnector(),
		connectors.NewHTTPConnector(),
	}
}
