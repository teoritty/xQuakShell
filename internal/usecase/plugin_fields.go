package usecase

import (
	"context"
	"fmt"
	"strconv"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

const pluginSecretRefPrefix = "secret:"

// PluginFieldsService validates and persists plugin connection fields in the vault.
type PluginFieldsService struct {
	vault    domain.VaultRepository
	registry *PluginRegistry
}

// NewPluginFieldsService creates a plugin fields service.
func NewPluginFieldsService(vault domain.VaultRepository, registry *PluginRegistry) *PluginFieldsService {
	return &PluginFieldsService{vault: vault, registry: registry}
}

// SavePluginFields validates and atomically stores plugin field values for a connection.
func (s *PluginFieldsService) SavePluginFields(ctx context.Context, conn *domain.Connection, incoming map[string]string) error {
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	if conn.GetProtocol() == domain.ProtocolSSH {
		conn.PluginFields = nil
		return nil
	}
	if len(incoming) == 0 && len(conn.PluginFields) == 0 {
		return nil
	}

	protoDef, _, err := s.registry.ProtocolDefForConnection(conn.GetProtocol())
	if err != nil {
		return err
	}
	fieldDefs := protoDef.GetFlatFields()

	snapshot := make(map[string]string, len(conn.PluginFields)+len(incoming))
	for k, v := range conn.PluginFields {
		snapshot[k] = v
	}
	for k, v := range incoming {
		snapshot[k] = v
	}

	visible := make(map[string]bool, len(fieldDefs))
	for i := range fieldDefs {
		visible[fieldDefs[i].ID] = domainplugin.IsFieldVisible(fieldDefs[i], snapshot)
	}

	resolved := make(map[string]string, len(incoming))
	secretsToStore := make(map[string][]byte)
	secretsToDelete := make([]string, 0)

	for id, value := range incoming {
		def := findFieldDef(fieldDefs, id)
		if def == nil {
			return fmt.Errorf("field %q not declared in protocol %q manifest", id, conn.GetProtocol())
		}
		if !visible[id] {
			continue
		}
		if err := validateFieldValue(value, def); err != nil {
			return fmt.Errorf("validation failed for field %q: %w", id, err)
		}

		if def.Secret {
			secretRef := pluginSecretRef(conn.ID, def.ID)
			if value == "" {
				secretsToDelete = append(secretsToDelete, secretRef)
				continue
			}
			secretsToStore[secretRef] = []byte(value)
			resolved[id] = secretRef
			continue
		}
		resolved[id] = value
	}

	for id, stored := range conn.PluginFields {
		def := protoDef.FlatFields[id]
		if def == nil {
			continue
		}
		if _, sent := incoming[id]; sent {
			continue
		}
		if !visible[id] {
			if def.Secret {
				secretsToDelete = append(secretsToDelete, stored)
			}
			continue
		}
		resolved[id] = stored
	}

	err = s.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		if data.PluginSecrets == nil {
			data.PluginSecrets = make(map[string][]byte)
		}
		for _, ref := range secretsToDelete {
			delete(data.PluginSecrets, ref)
		}
		for ref, value := range secretsToStore {
			data.PluginSecrets[ref] = append([]byte(nil), value...)
		}
		return updateConnectionPluginFieldsLocked(data, conn.ID, resolved)
	})
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		conn.PluginFields = nil
	} else {
		conn.PluginFields = cloneStringMap(resolved)
	}
	return nil
}

// ResolvePluginFields decrypts secret refs and returns plaintext values for session.connect.
func (s *PluginFieldsService) ResolvePluginFields(ctx context.Context, conn *domain.Connection, protoDef *domainplugin.ProtocolDef) (map[string]string, error) {
	if conn == nil || protoDef == nil || len(conn.PluginFields) == 0 {
		return map[string]string{}, nil
	}

	data, err := s.vault.GetData()
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]string, len(conn.PluginFields))
	for id, stored := range conn.PluginFields {
		def := protoDef.FlatFields[id]
		if def == nil {
			continue
		}

		if def.Secret {
			ciphertext, ok := data.PluginSecrets[stored]
			if !ok {
				return nil, fmt.Errorf("resolve secret for field %q: secret not found", id)
			}
			resolved[def.ID] = string(ciphertext)
		} else {
			resolved[def.ID] = stored
		}
	}
	return resolved, nil
}

func updateConnectionPluginFieldsLocked(data *domain.VaultData, connID string, fields map[string]string) error {
	for i := range data.Connections {
		if data.Connections[i].ID != connID {
			continue
		}
		if len(fields) == 0 {
			data.Connections[i].PluginFields = nil
		} else {
			data.Connections[i].PluginFields = cloneStringMap(fields)
		}
		return nil
	}
	return fmt.Errorf("connection %s: %w", connID, domain.ErrConnectionNotFound)
}

func pluginSecretRef(connID, fieldID string) string {
	return pluginSecretRefPrefix + connID + "." + fieldID
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func findFieldDef(fieldDefs []domainplugin.FieldDef, id string) *domainplugin.FieldDef {
	for i := range fieldDefs {
		if fieldDefs[i].ID == id {
			return &fieldDefs[i]
		}
	}
	return nil
}

func validateFieldValue(value string, def *domainplugin.FieldDef) error {
	if def.Required && value == "" {
		return fmt.Errorf("field is required")
	}
	if value == "" {
		return nil
	}

	if def.Type == domainplugin.FieldTypeCheckbox {
		if value != "true" && value != "false" {
			return fmt.Errorf("checkbox value must be 'true' or 'false', got %q", value)
		}
		return nil
	}

	if def.Type == domainplugin.FieldTypeTextarea && def.Validation != nil {
		if def.Validation.MaxSizeBytes > 0 && len(value) > def.Validation.MaxSizeBytes {
			return fmt.Errorf("value exceeds max size %d bytes", def.Validation.MaxSizeBytes)
		}
	}

	if def.Validation != nil {
		if def.Validation.MinLength > 0 && len(value) < def.Validation.MinLength {
			return fmt.Errorf("value length %d is less than minimum %d", len(value), def.Validation.MinLength)
		}
		if def.Validation.MaxLength > 0 && len(value) > def.Validation.MaxLength {
			return fmt.Errorf("value length %d exceeds maximum %d", len(value), def.Validation.MaxLength)
		}
		if def.Validation.Pattern != "" {
			re := def.Validation.CompiledPattern()
			if re == nil {
				return fmt.Errorf("field pattern not compiled")
			}
			if !re.MatchString(value) {
				return fmt.Errorf("value does not match pattern %q", def.Validation.Pattern)
			}
		}
		if def.Type == domainplugin.FieldTypeNumber {
			numValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid number format")
			}
			if def.Validation.Min != nil && numValue < *def.Validation.Min {
				return fmt.Errorf("value %v is less than minimum %v", numValue, *def.Validation.Min)
			}
			if def.Validation.Max != nil && numValue > *def.Validation.Max {
				return fmt.Errorf("value %v exceeds maximum %v", numValue, *def.Validation.Max)
			}
		}
	}

	if def.Type == domainplugin.FieldTypeSelect {
		found := false
		for _, opt := range def.Options {
			if opt.Value == value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value %q is not in allowed options", value)
		}
	}

	return nil
}
