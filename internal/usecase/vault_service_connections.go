package usecase

import (
	"context"

	"ssh-client/internal/domain"
)

// GetAllConnections returns all connections in the vault.
func (s *VaultService) GetAllConnections(ctx context.Context) ([]domain.Connection, error) {
	return s.connRepo.GetAllConnections(ctx)
}

// GetConnection returns one connection by ID.
func (s *VaultService) GetConnection(ctx context.Context, id string) (*domain.Connection, error) {
	return s.connRepo.GetByID(ctx, id)
}

// SaveConnection creates or updates a connection, persists plugin fields, reloads the saved
// record, and triggers an immediate ping when host and port are available.
func (s *VaultService) SaveConnection(ctx context.Context, conn *domain.Connection, incomingPluginFields map[string]string) (*domain.Connection, error) {
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

// DeleteConnection removes a connection by ID.
func (s *VaultService) DeleteConnection(ctx context.Context, id string) error {
	return s.connRepo.Delete(ctx, id)
}

// MoveConnections moves connections to a target folder.
func (s *VaultService) MoveConnections(ctx context.Context, connectionIDs []string, targetFolderID string) error {
	return s.connRepo.MoveToFolder(ctx, connectionIDs, targetFolderID)
}

// MoveFolder changes a folder's parent.
func (s *VaultService) MoveFolder(ctx context.Context, folderID, targetParentID string) error {
	return s.connRepo.MoveFolder(ctx, folderID, targetParentID)
}

// ReorderConnections updates the order of connections within a folder.
func (s *VaultService) ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error {
	return s.connRepo.ReorderConnections(ctx, connectionIDs, folderID)
}

// ReorderFolders updates the order of folders under a parent.
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
