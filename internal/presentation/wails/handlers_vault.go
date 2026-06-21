package wails

import (
	"context"
	"encoding/base64"
	"fmt"

	infraputty "ssh-client/internal/infra/putty"

	"ssh-client/internal/domain"
)

// --- Folders ---

// GetFolders returns all folders.
func (a *AppAPI) GetFolders() ([]FolderDTO, error) {
	fs, err := a.connRepo.GetAllFolders(context.Background())
	if err != nil {
		return nil, err
	}
	return FoldersToDTO(fs), nil
}

// SaveFolder creates or updates a folder.
func (a *AppAPI) SaveFolder(dto FolderDTO) (FolderDTO, error) {
	f := DTOToFolder(dto)
	if err := a.connRepo.SaveFolder(context.Background(), &f); err != nil {
		return FolderDTO{}, err
	}
	return FolderToDTO(f), nil
}

// DeleteFolder removes a folder, its descendant folders, and all connections inside that subtree.
func (a *AppAPI) DeleteFolder(id string) error {
	return a.connRepo.DeleteFolder(context.Background(), id)
}

// --- Connections ---

// GetAllConnections returns all connections.
func (a *AppAPI) GetAllConnections() ([]ConnectionDTO, error) {
	cs, err := a.connRepo.GetAllConnections(context.Background())
	if err != nil {
		return nil, err
	}
	return ConnectionsToDTO(cs), nil
}

// SaveConnection creates or updates a connection.
func (a *AppAPI) SaveConnection(dto ConnectionDTO) (ConnectionDTO, error) {
	c := DTOToConnection(dto)
	if err := a.connRepo.Save(context.Background(), &c); err != nil {
		return ConnectionDTO{}, err
	}
	if a.pingMgr != nil {
		if h := c.EffectiveHost(); h != "" && c.EffectivePort() > 0 {
			a.pingMgr.PingSingle(c.ID, h, c.EffectivePort())
		}
	}
	return ConnectionToDTO(c), nil
}

// DeleteConnection removes a connection by ID.
func (a *AppAPI) DeleteConnection(id string) error {
	return a.connRepo.Delete(context.Background(), id)
}

// MoveConnections moves connections to a target folder.
func (a *AppAPI) MoveConnections(connectionIDs []string, targetFolderID string) error {
	return a.connRepo.MoveToFolder(context.Background(), connectionIDs, targetFolderID)
}

// MoveFolder changes a folder's parent.
func (a *AppAPI) MoveFolder(folderID, targetParentID string) error {
	return a.connRepo.MoveFolder(context.Background(), folderID, targetParentID)
}

// ReorderConnections updates the order of connections within a folder.
func (a *AppAPI) ReorderConnections(connectionIDs []string, folderID string) error {
	return a.connRepo.ReorderConnections(context.Background(), connectionIDs, folderID)
}

// ReorderFolders updates the order of folders under a parent.
func (a *AppAPI) ReorderFolders(folderIDs []string, parentID string) error {
	return a.connRepo.ReorderFolders(context.Background(), folderIDs, parentID)
}

// --- Passwords ---

// ImportPassword stores a password in the vault and returns its ID.
func (a *AppAPI) ImportPassword(password, label string) (string, error) {
	return a.passwordRepo.Import(context.Background(), []byte(password), label)
}

// DeletePassword removes a password from the vault.
func (a *AppAPI) DeletePassword(id string) error {
	return a.passwordRepo.Delete(context.Background(), id)
}

// --- VPN Profiles ---

// ImportVPNProfile stores a VPN config in the vault and returns the profile ID.
func (a *AppAPI) ImportVPNProfile(configBase64, protocol, label string) (string, error) {
	configBlob, err := base64.StdEncoding.DecodeString(configBase64)
	if err != nil {
		return "", fmt.Errorf("invalid base64 config: %w", err)
	}
	prof := &domain.VPNProfile{
		Label:      label,
		Protocol:   domain.VPNProtocol(protocol),
		ConfigBlob: configBlob,
	}
	if err := a.vpnProfileRepo.Save(context.Background(), prof); err != nil {
		return "", err
	}
	return prof.ID, nil
}

// DeleteVPNProfile removes a VPN profile from the vault.
func (a *AppAPI) DeleteVPNProfile(id string) error {
	return a.vpnProfileRepo.Delete(context.Background(), id)
}

// GetVPNProfile returns a VPN profile by ID.
func (a *AppAPI) GetVPNProfile(id string) (VPNProfileDTO, error) {
	prof, err := a.vpnProfileRepo.Get(context.Background(), id)
	if err != nil {
		return VPNProfileDTO{}, err
	}
	return VPNProfileToDTO(prof), nil
}

// GetVPNProfiles returns all VPN profiles.
func (a *AppAPI) GetVPNProfiles() ([]VPNProfileDTO, error) {
	profiles, err := a.vpnProfileRepo.GetAll(context.Background())
	if err != nil {
		return nil, err
	}
	return VPNProfilesToDTO(profiles), nil
}

// --- Identities ---

// GetIdentities returns metadata for all SSH identities.
func (a *AppAPI) GetIdentities() ([]IdentityDTO, error) {
	ids, err := a.identRepo.GetAll(context.Background())
	if err != nil {
		return nil, err
	}
	return IdentitiesToDTO(ids), nil
}

// ImportIdentity imports a PEM private key (base64-encoded) into the vault.
// Returns the new identity ID.
func (a *AppAPI) ImportIdentity(pemBase64, comment string) (string, error) {
	pemData, err := base64.StdEncoding.DecodeString(pemBase64)
	if err != nil {
		return "", fmt.Errorf("decode pem base64: %w", err)
	}
	identity, err := a.identRepo.Import(context.Background(), pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// ImportPuTTYPPK imports a PuTTY .ppk file (base64-encoded content) into the vault as an identity.
// passphrase is required if the PPK is encrypted.
func (a *AppAPI) ImportPuTTYPPK(ppkBase64, passphrase string) (string, error) {
	ppkData, err := base64.StdEncoding.DecodeString(ppkBase64)
	if err != nil {
		return "", fmt.Errorf("decode ppk base64: %w", err)
	}
	pemData, comment, err := infraputty.PPKToPEM(ppkData, passphrase)
	if err != nil {
		return "", err
	}
	if comment == "" {
		comment = "PuTTY import"
	}
	identity, err := a.identRepo.Import(context.Background(), pemData, comment)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

// ImportPuTTYReg parses a PuTTY .reg file and returns session previews.
func (a *AppAPI) ImportPuTTYReg(regContent string) ([]PuTTYSessionDTO, error) {
	sessions, err := infraputty.ParsePuTTYReg(regContent)
	if err != nil {
		return nil, err
	}
	return PuTTYSessionsToDTO(sessions), nil
}

// ImportPuTTYRegAsConnections parses a PuTTY .reg file and creates connections in the given folder.
func (a *AppAPI) ImportPuTTYRegAsConnections(regContent, folderID string) ([]ConnectionDTO, error) {
	sessions, err := infraputty.ParsePuTTYReg(regContent)
	if err != nil {
		return nil, err
	}
	var result []ConnectionDTO
	for i, s := range sessions {
		if s.HostName == "" {
			continue
		}
		conn := s.ToConnection(folderID, i)
		conn.ID = ""
		if err := a.connRepo.Save(context.Background(), &conn); err != nil {
			return result, fmt.Errorf("save session %s: %w", s.Name, err)
		}
		result = append(result, ConnectionToDTO(conn))
	}
	return result, nil
}
