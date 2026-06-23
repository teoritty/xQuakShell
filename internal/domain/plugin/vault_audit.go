package plugin

import (
	"context"
	"time"
)

// VaultAccessMethod identifies a successful vault RPC audited by the host.
type VaultAccessMethod string

const (
	VaultAccessGetConnection VaultAccessMethod = "vault.getConnection"
	VaultAccessGetSecret     VaultAccessMethod = "vault.getSecret"
)

// VaultAccessEvent records a successful plugin vault read (no secret values).
type VaultAccessEvent struct {
	Timestamp    time.Time
	PluginID     string
	ConnectionID string
	Method       VaultAccessMethod
	Field        string // set for vault.getSecret only
}

// VaultAccessAuditLogger persists immutable audit records for plugin vault access.
type VaultAccessAuditLogger interface {
	RecordVaultAccess(ctx context.Context, event VaultAccessEvent) error
}
