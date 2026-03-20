package persistence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"ssh-client/internal/domain"
)

// ConnectionRepo implements domain.ConnectionRepository using the vault as backing store.
type ConnectionRepo struct {
	vault domain.VaultRepository
}

// NewConnectionRepo creates a ConnectionRepo backed by the given VaultRepository.
func NewConnectionRepo(v domain.VaultRepository) *ConnectionRepo {
	return &ConnectionRepo{vault: v}
}

// GetAllFolders returns every folder stored in the vault.
func (r *ConnectionRepo) GetAllFolders(ctx context.Context) ([]domain.ConnectionFolder, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get folders: %w", err)
	}
	result := make([]domain.ConnectionFolder, len(data.Folders))
	copy(result, data.Folders)
	return result, nil
}

// SaveFolder creates or updates a folder in the vault.
func (r *ConnectionRepo) SaveFolder(ctx context.Context, f *domain.ConnectionFolder) error {
	if err := f.Validate(); err != nil {
		return err
	}

	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("save folder get data: %w", err)
	}

	if f.ID == "" {
		f.ID = uuid.New().String()
		data.Folders = append(data.Folders, *f)
	} else {
		found := false
		for i := range data.Folders {
			if data.Folders[i].ID == f.ID {
				data.Folders[i] = *f
				found = true
				break
			}
		}
		if !found {
			data.Folders = append(data.Folders, *f)
		}
	}

	return r.vault.SaveData(ctx, data)
}

// DeleteFolder removes a folder by ID. Connections in the folder are moved to root.
func (r *ConnectionRepo) DeleteFolder(ctx context.Context, id string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("delete folder get data: %w", err)
	}

	for i := range data.Connections {
		if data.Connections[i].FolderID == id {
			data.Connections[i].FolderID = ""
		}
	}

	filtered := make([]domain.ConnectionFolder, 0, len(data.Folders))
	for _, f := range data.Folders {
		if f.ID != id {
			filtered = append(filtered, f)
		}
	}
	data.Folders = filtered

	return r.vault.SaveData(ctx, data)
}

// GetAllConnections returns all connections regardless of folder.
func (r *ConnectionRepo) GetAllConnections(ctx context.Context) ([]domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get all connections: %w", err)
	}
	result := make([]domain.Connection, len(data.Connections))
	copy(result, data.Connections)
	return result, nil
}

// GetByFolder returns connections belonging to a specific folder.
func (r *ConnectionRepo) GetByFolder(ctx context.Context, folderID string) ([]domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get by folder: %w", err)
	}

	var result []domain.Connection
	for _, c := range data.Connections {
		if c.FolderID == folderID {
			result = append(result, c)
		}
	}
	return result, nil
}

// GetByID returns a single connection by its ID.
func (r *ConnectionRepo) GetByID(ctx context.Context, id string) (*domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get by id: %w", err)
	}

	for i := range data.Connections {
		if data.Connections[i].ID == id {
			c := data.Connections[i]
			return &c, nil
		}
	}
	return nil, fmt.Errorf("connection %s: %w", id, domain.ErrConnectionNotFound)
}

// Save creates or updates a connection in the vault.
func (r *ConnectionRepo) Save(ctx context.Context, c *domain.Connection) error {
	c.WithDefaults()
	if err := c.Validate(); err != nil {
		return err
	}

	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("save connection get data: %w", err)
	}

	if c.ID == "" {
		c.ID = uuid.New().String()
		data.Connections = append(data.Connections, *c)
	} else {
		found := false
		for i := range data.Connections {
			if data.Connections[i].ID == c.ID {
				data.Connections[i] = *c
				found = true
				break
			}
		}
		if !found {
			data.Connections = append(data.Connections, *c)
		}
	}

	return r.vault.SaveData(ctx, data)
}

// Delete removes a connection by ID.
func (r *ConnectionRepo) Delete(ctx context.Context, id string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("delete connection get data: %w", err)
	}

	filtered := make([]domain.Connection, 0, len(data.Connections))
	for _, c := range data.Connections {
		if c.ID != id {
			filtered = append(filtered, c)
		}
	}
	data.Connections = filtered

	return r.vault.SaveData(ctx, data)
}

// MoveToFolder moves the given connections into a target folder.
func (r *ConnectionRepo) MoveToFolder(ctx context.Context, connectionIDs []string, folderID string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("move to folder get data: %w", err)
	}

	idSet := make(map[string]struct{}, len(connectionIDs))
	for _, id := range connectionIDs {
		idSet[id] = struct{}{}
	}

	for i := range data.Connections {
		if _, ok := idSet[data.Connections[i].ID]; ok {
			data.Connections[i].FolderID = folderID
		}
	}

	return r.vault.SaveData(ctx, data)
}

// MoveFolder changes a folder's parent. targetParentID="" moves to root.
// Returns ErrCircularFolder if the move would create a cycle.
func (r *ConnectionRepo) MoveFolder(ctx context.Context, folderID, targetParentID string) error {
	if folderID == targetParentID {
		return fmt.Errorf("cannot move folder into itself: %w", domain.ErrCircularFolder)
	}

	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("move folder get data: %w", err)
	}

	folderIndex := -1
	folderMap := make(map[string]*domain.ConnectionFolder, len(data.Folders))
	for i := range data.Folders {
		folderMap[data.Folders[i].ID] = &data.Folders[i]
		if data.Folders[i].ID == folderID {
			folderIndex = i
		}
	}

	if folderIndex < 0 {
		return fmt.Errorf("folder %s: %w", folderID, domain.ErrFolderNotFound)
	}

	if targetParentID != "" {
		if _, ok := folderMap[targetParentID]; !ok {
			return fmt.Errorf("target folder %s: %w", targetParentID, domain.ErrFolderNotFound)
		}
		if isDescendant(folderMap, targetParentID, folderID) {
			return fmt.Errorf("folder %s is ancestor of %s: %w", folderID, targetParentID, domain.ErrCircularFolder)
		}
	}

	data.Folders[folderIndex].ParentID = targetParentID
	return r.vault.SaveData(ctx, data)
}

// ReorderConnections updates Order for connections in the folder to match the given order.
func (r *ConnectionRepo) ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("reorder connections get data: %w", err)
	}

	idToOrder := make(map[string]int)
	for i, id := range connectionIDs {
		idToOrder[id] = i
	}

	for i := range data.Connections {
		if data.Connections[i].FolderID != folderID {
			continue
		}
		if ord, ok := idToOrder[data.Connections[i].ID]; ok {
			data.Connections[i].Order = ord
		}
	}

	return r.vault.SaveData(ctx, data)
}

// ReorderFolders updates Order for folders under parentID to match the given order.
func (r *ConnectionRepo) ReorderFolders(ctx context.Context, folderIDs []string, parentID string) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("reorder folders get data: %w", err)
	}

	idToOrder := make(map[string]int)
	for i, id := range folderIDs {
		idToOrder[id] = i
	}

	for i := range data.Folders {
		if data.Folders[i].ParentID != parentID {
			continue
		}
		if ord, ok := idToOrder[data.Folders[i].ID]; ok {
			data.Folders[i].Order = ord
		}
	}

	return r.vault.SaveData(ctx, data)
}

// isDescendant returns true if candidate is a descendant of ancestor in the folder tree.
func isDescendant(folders map[string]*domain.ConnectionFolder, candidate, ancestor string) bool {
	visited := make(map[string]bool)
	current := candidate
	for current != "" {
		if current == ancestor {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true
		f, ok := folders[current]
		if !ok {
			return false
		}
		current = f.ParentID
	}
	return false
}
