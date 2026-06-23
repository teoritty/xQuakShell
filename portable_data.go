package main

import "ssh-client/internal/infra/portable"

// portableDataRoot returns <exe>/data for vault, audit log, and plugins (ADR-006).
func portableDataRoot() string {
	return portable.Default.DataRoot()
}
