package auditlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// NDJSONVaultAuditLogger appends plugin vault access events to an immutable NDJSON file.
type NDJSONVaultAuditLogger struct {
	path string
	mu   sync.Mutex
}

type vaultAuditRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	PluginID     string    `json:"pluginId"`
	ConnectionID string    `json:"connectionId"`
	Method       string    `json:"method"`
	Field        string    `json:"field,omitempty"`
}

// NewNDJSONVaultAuditLogger creates an append-only audit log under dataRoot/audit/.
func NewNDJSONVaultAuditLogger(dataRoot string) (*NDJSONVaultAuditLogger, error) {
	dir := filepath.Join(dataRoot, "audit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create vault audit dir: %w", err)
	}
	path := filepath.Join(dir, "plugin-vault.ndjson")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open vault audit log: %w", err)
	}
	_ = f.Close()
	return &NDJSONVaultAuditLogger{path: path}, nil
}

// RecordVaultAccess implements domainplugin.VaultAccessAuditLogger.
func (l *NDJSONVaultAuditLogger) RecordVaultAccess(_ context.Context, event domainplugin.VaultAccessEvent) error {
	if l == nil {
		return domainplugin.ErrVaultAuditFailed
	}
	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	record := vaultAuditRecord{
		Timestamp:    ts.UTC(),
		PluginID:     event.PluginID,
		ConnectionID: event.ConnectionID,
		Method:       string(event.Method),
		Field:        event.Field,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("%w: marshal audit record: %v", domainplugin.ErrVaultAuditFailed, err)
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("%w: %v", domainplugin.ErrVaultAuditFailed, err)
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("%w: %v", domainplugin.ErrVaultAuditFailed, err)
	}
	return nil
}
