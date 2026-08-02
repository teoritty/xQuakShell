package wails

import (
	"context"
	"encoding/base64"
	"fmt"
)

// --- Folders ---

func (a *AppAPI) GetFolders() ([]FolderDTO, error) {
	fs, err := a.vaultSvc.GetAllFolders(context.Background())
	if err != nil {
		return nil, err
	}
	return FoldersToDTO(fs), nil
}

func (a *AppAPI) SaveFolder(dto FolderDTO) (FolderDTO, error) {
	f := DTOToFolder(dto)
	if err := a.vaultSvc.SaveFolder(context.Background(), &f); err != nil {
		return FolderDTO{}, err
	}
	return FolderToDTO(f), nil
}

func (a *AppAPI) DeleteFolder(id string) error {
	return a.vaultSvc.DeleteFolder(context.Background(), id)
}

// --- Connections ---

func (a *AppAPI) GetAllConnections() ([]ConnectionDTO, error) {
	cs, err := a.vaultSvc.GetAllConnections(context.Background())
	if err != nil {
		return nil, err
	}
	return ConnectionsToDTO(cs), nil
}

func (a *AppAPI) SaveConnection(dto ConnectionDTO) (ConnectionDTO, error) {
	c := DTOToConnection(dto)
	saved, err := a.vaultSvc.SaveConnection(context.Background(), &c, cloneStringMap(dto.PluginFields))
	if err != nil {
		return ConnectionDTO{}, err
	}
	return ConnectionToDTO(*saved), nil
}

func (a *AppAPI) DeleteConnection(id string) error {
	return a.vaultSvc.DeleteConnection(context.Background(), id)
}

func (a *AppAPI) MoveConnections(connectionIDs []string, targetFolderID string) error {
	return a.vaultSvc.MoveConnections(context.Background(), connectionIDs, targetFolderID)
}

func (a *AppAPI) MoveFolder(folderID, targetParentID string) error {
	return a.vaultSvc.MoveFolder(context.Background(), folderID, targetParentID)
}

func (a *AppAPI) ReorderConnections(connectionIDs []string, folderID string) error {
	return a.vaultSvc.ReorderConnections(context.Background(), connectionIDs, folderID)
}

func (a *AppAPI) ReorderFolders(folderIDs []string, parentID string) error {
	return a.vaultSvc.ReorderFolders(context.Background(), folderIDs, parentID)
}

// --- Passwords ---

func (a *AppAPI) ImportPassword(password, label string) (string, error) {
	return a.vaultSvc.ImportPassword(context.Background(), []byte(password), label)
}

func (a *AppAPI) DeletePassword(id string) error {
	return a.vaultSvc.DeletePassword(context.Background(), id)
}

// --- Identities ---

func (a *AppAPI) GetIdentities() ([]IdentityDTO, error) {
	ids, err := a.vaultSvc.GetAllIdentities(context.Background())
	if err != nil {
		return nil, err
	}
	return IdentitiesToDTO(ids), nil
}

func (a *AppAPI) ImportIdentity(pemBase64, comment string) (string, error) {
	pemData, err := base64.StdEncoding.DecodeString(pemBase64)
	if err != nil {
		return "", fmt.Errorf("decode pem base64: %w", err)
	}
	identity, err := a.vaultSvc.ImportIdentity(context.Background(), pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

func (a *AppAPI) ImportPuTTYPPK(ppkBase64, passphrase string) (string, error) {
	if a.puttyImport == nil {
		return "", fmt.Errorf("putty import unavailable")
	}
	ppkData, err := base64.StdEncoding.DecodeString(ppkBase64)
	if err != nil {
		return "", fmt.Errorf("decode ppk base64: %w", err)
	}
	return a.puttyImport.ImportPPK(context.Background(), ppkData, passphrase)
}

func (a *AppAPI) ImportPuTTYReg(regContent string) ([]PuTTYSessionDTO, error) {
	if a.puttyImport == nil {
		return nil, fmt.Errorf("putty import unavailable")
	}
	sessions, err := a.puttyImport.ParseReg(regContent)
	if err != nil {
		return nil, err
	}
	return puttySessionsToDTO(sessions), nil
}

func (a *AppAPI) ImportPuTTYRegAsConnections(regContent, folderID string) ([]ConnectionDTO, error) {
	if a.puttyImport == nil {
		return nil, fmt.Errorf("putty import unavailable")
	}
	connections, err := a.puttyImport.ImportRegAsConnections(context.Background(), regContent, folderID)
	if err != nil {
		return nil, err
	}
	result := make([]ConnectionDTO, 0, len(connections))
	for _, conn := range connections {
		result = append(result, ConnectionToDTO(conn))
	}
	return result, nil
}
