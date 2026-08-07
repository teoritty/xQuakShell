package wails

import (
	"errors"
	"fmt"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// maxImportAliases bounds one import request. The frontend selects from a
// preview that is itself capped, so a larger list means a malformed or
// hostile call rather than a user with very many hosts.
const maxImportAliases = 5000

// GetSSHConfigDefaultPath returns the user's conventional ssh_config path, or
// an empty string when there is none. A missing config is not an error: the
// dialog opens with an empty field and a hint.
func (a *AppAPI) GetSSHConfigDefaultPath() (string, error) {
	if a.sshConfigImport == nil {
		return "", errSSHConfigUnavailable()
	}
	path, err := a.sshConfigImport.DefaultPath()
	if err != nil {
		return "", sshConfigUserError(err)
	}
	return path, nil
}

// PreviewSSHConfig parses a config file and reports the hosts it offers,
// without writing anything to the vault.
func (a *AppAPI) PreviewSSHConfig(path string) (SSHConfigPreviewDTO, error) {
	if a.sshConfigImport == nil {
		return SSHConfigPreviewDTO{}, errSSHConfigUnavailable()
	}
	if strings.TrimSpace(path) == "" {
		return SSHConfigPreviewDTO{}, fmt.Errorf("select an SSH config file first")
	}
	preview, err := a.sshConfigImport.Preview(a.reqCtx(), path)
	if err != nil {
		return SSHConfigPreviewDTO{}, sshConfigUserError(err)
	}
	return sshConfigPreviewToDTO(preview), nil
}

// ImportSSHConfig creates vault connections for the selected hosts.
//
// The path is re-parsed by the use case, so this handler forwards a file
// reference and a selection — never file contents and never a key path.
func (a *AppAPI) ImportSSHConfig(path string, aliases []string, folderID string, importKeys bool) (SSHConfigImportResultDTO, error) {
	if a.sshConfigImport == nil {
		return SSHConfigImportResultDTO{}, errSSHConfigUnavailable()
	}
	if strings.TrimSpace(path) == "" {
		return SSHConfigImportResultDTO{}, fmt.Errorf("select an SSH config file first")
	}
	if len(aliases) == 0 {
		return SSHConfigImportResultDTO{}, fmt.Errorf("select at least one host to import")
	}
	if len(aliases) > maxImportAliases {
		return SSHConfigImportResultDTO{}, fmt.Errorf("too many hosts selected")
	}

	result, err := a.sshConfigImport.Import(a.reqCtx(), usecase.SSHConfigImportRequest{
		Path:       path,
		Aliases:    aliases,
		FolderID:   folderID,
		ImportKeys: importKeys,
	})
	dto := sshConfigImportResultToDTO(result)
	if err != nil {
		// Partial progress is returned alongside the error so the dialog can
		// say how many connections were actually created before it stopped.
		return dto, sshConfigUserError(err)
	}
	return dto, nil
}

func errSSHConfigUnavailable() error {
	return fmt.Errorf("SSH config import is unavailable")
}

// sshConfigUserError maps internal errors onto messages that are safe and
// useful to show. Anything unrecognised becomes a generic sentence rather
// than the underlying text, which may name paths or OS internals.
func sshConfigUserError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrVaultLocked):
		return fmt.Errorf("vault is locked")
	case errors.Is(err, domain.ErrSSHConfigNotFound):
		return fmt.Errorf("SSH config file not found")
	case errors.Is(err, domain.ErrSSHConfigTooLarge):
		return fmt.Errorf("SSH config file is too large to read")
	case errors.Is(err, domain.ErrSSHConfigUnreadable):
		return fmt.Errorf("SSH config file could not be read")
	case errors.Is(err, domain.ErrInvalidConnectionConfig):
		return fmt.Errorf("SSH config contains an invalid connection")
	default:
		return fmt.Errorf("SSH config import failed")
	}
}
