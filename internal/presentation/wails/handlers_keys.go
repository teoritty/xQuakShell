package wails

import (
	"encoding/base64"
	"fmt"

	"xquakshell/internal/domain"
)

// --- Listing ---

// GetKeys returns every stored key's metadata. It never carries private material: see IdentityDTO.
func (a *AppAPI) GetKeys() ([]IdentityDTO, error) {
	identities, err := a.keys.List(a.reqCtx())
	if err != nil {
		return nil, err
	}
	return IdentitiesToDTO(identities), nil
}

// GetKeyUsages lists the connections referring to a key, so the UI can explain a refused delete.
func (a *AppAPI) GetKeyUsages(id string) ([]KeyUsageDTO, error) {
	usages, err := a.keys.Usages(a.reqCtx(), id)
	if err != nil {
		return nil, err
	}
	return KeyUsagesToDTO(usages), nil
}

// --- Creating ---

// GenerateKey creates a key inside the vault. An empty passphrase puts it under the vault's own
// protection rather than leaving it unprotected.
func (a *AppAPI) GenerateKey(algorithm string, bits int, comment, passphrase string, opts KeyOptionsDTO) (IdentityDTO, error) {
	spec := domain.GeneratedKeySpec{Algorithm: algorithm, Bits: bits, Comment: comment}
	identity, err := a.keys.Generate(a.reqCtx(), spec, passphrase, DTOToKeyOptions(opts))
	if err != nil {
		return IdentityDTO{}, err
	}
	return IdentityToDTO(*identity), nil
}

// ImportKey stores a key the user supplied. The PEM arrives base64-encoded because Wails moves it
// as JSON, where raw key bytes would be mangled by newline and encoding handling.
func (a *AppAPI) ImportKey(pemBase64, passphrase, comment string, opts KeyOptionsDTO) (IdentityDTO, error) {
	pemData, err := base64.StdEncoding.DecodeString(pemBase64)
	if err != nil {
		return IdentityDTO{}, fmt.Errorf("invalid key encoding")
	}
	identity, err := a.keys.Import(a.reqCtx(), pemData, passphrase, comment, DTOToKeyOptions(opts))
	if err != nil {
		return IdentityDTO{}, err
	}
	return IdentityToDTO(*identity), nil
}

// --- Editing ---

// RenameKey changes a key's label.
func (a *AppAPI) RenameKey(id, comment string) error {
	return a.keys.Rename(a.reqCtx(), id, comment)
}

// SetKeyPolicy updates caching, plugin access and the export flag.
func (a *AppAPI) SetKeyPolicy(id string, opts KeyOptionsDTO) error {
	return a.keys.SetPolicy(a.reqCtx(), id, DTOToKeyOptions(opts))
}

// ChangeKeyPassphrase re-wraps a key. An empty newPassphrase moves it under the vault's
// protection; it never leaves the key unprotected.
func (a *AppAPI) ChangeKeyPassphrase(id, oldPassphrase, newPassphrase string) error {
	return a.keys.ChangePassphrase(a.reqCtx(), id, oldPassphrase, newPassphrase)
}

// DeleteKey removes a key, refusing while a connection still refers to it.
func (a *AppAPI) DeleteKey(id string) error {
	return a.keys.Delete(a.reqCtx(), id)
}

// --- Leaving the vault ---

// ExportKey returns the key as an OpenSSH file, base64-encoded for transport.
//
// The master password is verified here rather than being trusted from the frontend: the use case
// takes a boolean it will not second-guess, so the check has to happen on this side of the bridge
// or it does not happen at all. An empty exportPassphrase produces an unprotected file, which is
// the user's explicit choice on the export screen and is why this call is gated at all.
func (a *AppAPI) ExportKey(id, masterPassword, passphrase, exportPassphrase string) (string, error) {
	if err := a.verifyMasterPassword(masterPassword); err != nil {
		return "", err
	}
	out, err := a.keys.Export(a.reqCtx(), id, passphrase, exportPassphrase, true)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DeployKey appends a key's public half to the authorized_keys of an open session.
func (a *AppAPI) DeployKey(sessionID, identityID string) (DeployResultDTO, error) {
	result, err := a.keyDeploy.Deploy(a.reqCtx(), sessionID, identityID)
	if err != nil {
		return DeployResultDTO{}, err
	}
	return DeployResultDTO{Added: result.Added, AlreadyPresent: result.AlreadyPresent, Path: result.Path}, nil
}

// verifyMasterPassword re-authenticates the user without disturbing the open vault.
//
// An empty password is refused before reaching the vault: it is never a real answer, and letting
// it through would spend a full scrypt pass to say so.
func (a *AppAPI) verifyMasterPassword(masterPassword string) error {
	if masterPassword == "" {
		return domain.ErrVaultDecryptFailed
	}
	return a.vaultRepo.VerifyMasterPassword(a.reqCtx(), masterPassword)
}
