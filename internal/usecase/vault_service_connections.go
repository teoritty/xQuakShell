package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain"
)

func (s *VaultService) GetAllConnections(ctx context.Context) ([]domain.Connection, error) {
	return s.connRepo.GetAllConnections(ctx)
}

func (s *VaultService) GetConnection(ctx context.Context, id string) (*domain.Connection, error) {
	return s.connRepo.GetByID(ctx, id)
}

// SaveConnection creates or updates a connection, persists plugin fields, reloads the saved
// record, and triggers an immediate ping when host and port are available.
func (s *VaultService) SaveConnection(ctx context.Context, conn *domain.Connection, incomingPluginFields map[string]string) (*domain.Connection, error) {
	if conn != nil {
		prepared, err := prepareConnectionForwardRules(ctx, s.forwardRuleValidator, conn.ID, conn.ForwardRules)
		if err != nil {
			return nil, err
		}
		conn.ForwardRules = prepared
		if err := s.mergeStoredPluginFields(ctx, conn); err != nil {
			return nil, err
		}
	}
	if err := s.connRepo.Save(ctx, conn); err != nil {
		return nil, err
	}
	if s.pluginFields != nil {
		if err := s.pluginFields.SavePluginFields(ctx, conn, incomingPluginFields); err != nil {
			return nil, err
		}
	}
	saved, err := s.connRepo.GetByID(ctx, conn.ID)
	if err != nil {
		return nil, err
	}
	s.pingAfterConnectionSave(ctx, saved)
	return saved, nil
}

// mergeStoredPluginFields folds the plugin fields already on record into conn before conn is
// written over that record.
//
// A caller's payload is a partial statement about plugin fields, never the whole one. A secret
// already in the vault is never handed back to the UI, so nothing that saves a connection it read
// earlier - the tree's inline rename, any future partial save - can return that value. Without this
// merge the record is overwritten with the payload alone and the reference to the stored secret is
// gone, and SavePluginFields cannot repair it afterwards: it treats a field absent from the payload
// as "keep what is stored", and what is stored is conn, which was built from the payload.
//
// The final set is still SavePluginFields' decision. This only makes sure it decides against the
// real prior state instead of against a copy of its own input.
func (s *VaultService) mergeStoredPluginFields(ctx context.Context, conn *domain.Connection) error {
	if conn.ID == "" {
		return nil
	}
	stored, err := s.connRepo.GetByID(ctx, conn.ID)
	if err != nil {
		if errors.Is(err, domain.ErrConnectionNotFound) {
			return nil
		}
		return err
	}
	if len(stored.PluginFields) == 0 {
		return nil
	}
	merged := make(map[string]string, len(stored.PluginFields)+len(conn.PluginFields))
	for id, value := range stored.PluginFields {
		merged[id] = value
	}
	for id, value := range conn.PluginFields {
		merged[id] = value
	}
	conn.PluginFields = merged
	return nil
}

func (s *VaultService) DeleteConnection(ctx context.Context, id string) error {
	return s.connRepo.Delete(ctx, id)
}

func (s *VaultService) MoveConnections(ctx context.Context, connectionIDs []string, targetFolderID string) error {
	return s.connRepo.MoveToFolder(ctx, connectionIDs, targetFolderID)
}

func (s *VaultService) MoveFolder(ctx context.Context, folderID, targetParentID string) error {
	return s.connRepo.MoveFolder(ctx, folderID, targetParentID)
}

func (s *VaultService) ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error {
	return s.connRepo.ReorderConnections(ctx, connectionIDs, folderID)
}

func (s *VaultService) ReorderFolders(ctx context.Context, folderIDs []string, parentID string) error {
	return s.connRepo.ReorderFolders(ctx, folderIDs, parentID)
}

func (s *VaultService) pingAfterConnectionSave(ctx context.Context, conn *domain.Connection) {
	if s == nil || s.pingMgr == nil || conn == nil {
		return
	}
	host := conn.EffectiveHost()
	if host == "" {
		return
	}
	port := conn.EffectivePort(s.protocolLookup)
	if port <= 0 {
		return
	}
	s.pingMgr.PingSingle(ctx, conn.ID, host, port)
}
