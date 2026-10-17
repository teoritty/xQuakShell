package persistence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"xquakshell/internal/domain"
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
func (r *ConnectionRepo) GetAllFolders(_ context.Context) ([]domain.ConnectionFolder, error) {
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

	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		// An edit never changes the vault an item is in, so it cannot move one out of a team: an
		// existing folder keeps its vault. A new one is created in the vault the caller asked for,
		// which the presentation layer checked this account may write to, or in its parent's.
		requested := f.VaultID
		f.VaultID = ""
		if f.ID == "" {
			vault, err := vaultForNewItem(data.Folders, f.ParentID, requested)
			if err != nil {
				return err
			}
			f.ID, f.VaultID = uuid.New().String(), vault
			f.Order = orderBeforeSiblings(data.Folders, f.ParentID)
			data.Folders = append(data.Folders, *f)
			return nil
		}
		for i := range data.Folders {
			if data.Folders[i].ID == f.ID {
				f.VaultID = data.Folders[i].VaultID
				if err := requireSameVault(data.Folders, f.ParentID, f.VaultID); err != nil {
					return err
				}
				data.Folders[i] = *f
				return nil
			}
		}
		data.Folders = append(data.Folders, *f)
		return nil
	})
}

// folderVault reports the vault a folder is in, and whether the folder exists.
func folderVault(folders []domain.ConnectionFolder, id string) (string, bool) {
	for i := range folders {
		if folders[i].ID == id {
			return folders[i].VaultID, true
		}
	}
	return "", false
}

// vaultForNewItem is the vault a new item created under folder lands in: the folder's own, which a
// requested vault must agree with. The top level of a vault has no folder, so there the request
// alone decides.
func vaultForNewItem(folders []domain.ConnectionFolder, folder, requested string) (string, error) {
	vault, ok := folderVault(folders, folder)
	if folder == "" || !ok {
		return requested, nil
	}
	if requested != "" && requested != vault {
		return "", domain.ErrVaultMismatch
	}
	return vault, nil
}

// requireSameVault refuses to put an item of vault into a folder of another one. A folder that does
// not exist is left alone, as it always was: the tree shows such an item at the top level.
func requireSameVault(folders []domain.ConnectionFolder, folder, vault string) error {
	if got, ok := folderVault(folders, folder); folder != "" && ok && got != vault {
		return domain.ErrVaultMismatch
	}
	return nil
}

// orderBeforeSiblings returns an order that sorts a new folder ahead of the folders already under
// the same parent.
//
// A new folder used to be stored with the zero Order it arrived with, which put it wherever the
// tree's stable sort happened to place a tie - in practice directly after the first folder, since
// that one usually also has order zero. Landing in the middle of a list for no reason the user can
// see is worse than either end, and the top is where something just created should be: it is where
// the user is already looking, and it needs no scrolling to find.
//
// Negative orders are fine. Nothing validates the range, the tree only ever compares them, and the
// first explicit reorder renumbers the level from zero anyway.
func orderBeforeSiblings(folders []domain.ConnectionFolder, parentID string) int {
	lowest := 0
	for _, sibling := range folders {
		if sibling.ParentID == parentID && sibling.Order < lowest {
			lowest = sibling.Order
		}
	}
	return lowest - 1
}

// DeleteFolder removes a folder by ID and all descendant folders. Connections in those folders are removed.
func (r *ConnectionRepo) DeleteFolder(ctx context.Context, id string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		subtree := folderSubtreeIDs(data.Folders, id)

		keptConn := make([]domain.Connection, 0, len(data.Connections))
		for _, c := range data.Connections {
			if _, in := subtree[c.FolderID]; in {
				continue
			}
			keptConn = append(keptConn, c)
		}
		data.Connections = keptConn

		keptFolders := make([]domain.ConnectionFolder, 0, len(data.Folders))
		for _, f := range data.Folders {
			if _, in := subtree[f.ID]; in {
				continue
			}
			keptFolders = append(keptFolders, f)
		}
		data.Folders = keptFolders
		return nil
	})
}

// folderSubtreeIDs returns rootID and all descendant folder IDs (by ParentID chain).
func folderSubtreeIDs(folders []domain.ConnectionFolder, rootID string) map[string]struct{} {
	out := map[string]struct{}{rootID: {}}
	for {
		added := false
		for _, f := range folders {
			if f.ID == "" {
				continue
			}
			if _, has := out[f.ID]; has {
				continue
			}
			if _, ok := out[f.ParentID]; ok {
				out[f.ID] = struct{}{}
				added = true
			}
		}
		if !added {
			break
		}
	}
	return out
}

// GetAllConnections returns all connections regardless of folder.
func (r *ConnectionRepo) GetAllConnections(_ context.Context) ([]domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get all connections: %w", err)
	}
	result := make([]domain.Connection, len(data.Connections))
	for i := range data.Connections {
		result[i] = domain.CloneConnection(data.Connections[i])
	}
	return result, nil
}

// GetByFolder returns connections belonging to a specific folder.
func (r *ConnectionRepo) GetByFolder(_ context.Context, folderID string) ([]domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get by folder: %w", err)
	}

	var result []domain.Connection
	for _, c := range data.Connections {
		if c.FolderID == folderID {
			result = append(result, domain.CloneConnection(c))
		}
	}
	return result, nil
}

// GetByID returns a single connection by its ID.
func (r *ConnectionRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("get by id: %w", err)
	}

	for i := range data.Connections {
		if data.Connections[i].ID == id {
			c := domain.CloneConnection(data.Connections[i])
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

	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		for i := range c.JumpChain.Hops {
			if c.JumpChain.Hops[i].ID == "" {
				c.JumpChain.Hops[i].ID = uuid.New().String()
			}
		}

		// As for folders: an edit keeps the vault, and a new connection goes where it was asked to.
		requested := c.VaultID
		c.VaultID = ""
		if c.ID == "" {
			vault, err := vaultForNewItem(data.Folders, c.FolderID, requested)
			if err != nil {
				return err
			}
			c.ID, c.VaultID = uuid.New().String(), vault
			data.Connections = append(data.Connections, *c)
			return nil
		}
		for i := range data.Connections {
			if data.Connections[i].ID == c.ID {
				c.VaultID = data.Connections[i].VaultID
				if err := requireSameVault(data.Folders, c.FolderID, c.VaultID); err != nil {
					return err
				}
				data.Connections[i] = *c
				return nil
			}
		}
		data.Connections = append(data.Connections, *c)
		return nil
	})
}

// Delete removes a connection by ID.
func (r *ConnectionRepo) Delete(ctx context.Context, id string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		filtered := make([]domain.Connection, 0, len(data.Connections))
		for _, c := range data.Connections {
			if c.ID != id {
				filtered = append(filtered, c)
			}
		}
		data.Connections = filtered
		return nil
	})
}

// MoveToFolder moves the given connections into a target folder.
func (r *ConnectionRepo) MoveToFolder(ctx context.Context, connectionIDs []string, folderID string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		idSet := make(map[string]struct{}, len(connectionIDs))
		for _, id := range connectionIDs {
			idSet[id] = struct{}{}
		}
		// Every connection is checked before any moves: UpdateData keeps what a failed mutation
		// already changed, and half a batch moved is a state nobody asked for.
		for _, c := range data.Connections {
			if _, ok := idSet[c.ID]; ok {
				if err := requireSameVault(data.Folders, folderID, c.VaultID); err != nil {
					return err
				}
			}
		}

		for i := range data.Connections {
			if _, ok := idSet[data.Connections[i].ID]; ok {
				data.Connections[i].FolderID = folderID
			}
		}
		return nil
	})
}

// MoveFolder changes a folder's parent. targetParentID="" moves to root.
// Returns ErrCircularFolder if the move would create a cycle.
func (r *ConnectionRepo) MoveFolder(ctx context.Context, folderID, targetParentID string) error {
	if folderID == targetParentID {
		return fmt.Errorf("cannot move folder into itself: %w", domain.ErrCircularFolder)
	}

	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
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
		if err := requireSameVault(data.Folders, targetParentID, data.Folders[folderIndex].VaultID); err != nil {
			return err
		}

		data.Folders[folderIndex].ParentID = targetParentID
		return nil
	})
}

// ReorderConnections updates Order for connections in the folder to match the given order.
func (r *ConnectionRepo) ReorderConnections(ctx context.Context, connectionIDs []string, folderID string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
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
		return nil
	})
}

// ReorderFolders updates Order for folders under parentID to match the given order.
func (r *ConnectionRepo) ReorderFolders(ctx context.Context, folderIDs []string, parentID string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
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
		return nil
	})
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
