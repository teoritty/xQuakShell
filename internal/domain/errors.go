package domain

import "errors"

// ErrUnknownHost indicates the remote host key is not present in known_hosts.
var ErrUnknownHost = errors.New("unknown host: key not in known_hosts")

// ErrHostKeyMismatch indicates the remote host key differs from the one stored in known_hosts.
var ErrHostKeyMismatch = errors.New("host key mismatch: remote key differs from known_hosts")

// ErrInvalidConnectionConfig indicates a connection has invalid or missing fields.
var ErrInvalidConnectionConfig = errors.New("invalid connection configuration")

// ErrVaultLocked indicates an operation requires the vault to be unlocked first.
var ErrVaultLocked = errors.New("vault is locked")

// ErrVaultDecryptFailed indicates decryption of the vault failed (wrong master password or corrupted data).
var ErrVaultDecryptFailed = errors.New("vault decryption failed")

// ErrSessionNotFound indicates no active session exists with the given ID.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionAlreadyExists indicates a session for the given connection is already open.
var ErrSessionAlreadyExists = errors.New("session already exists for this connection")

// ErrIdentityNotFound indicates the requested SSH identity does not exist in the vault.
var ErrIdentityNotFound = errors.New("SSH identity not found in vault")

// ErrPassphraseRequired indicates an encrypted private key needs a passphrase to be parsed.
var ErrPassphraseRequired = errors.New("passphrase required for encrypted private key")

// ErrFolderNotEmpty indicates a folder still contains connections and cannot be deleted directly.
var ErrFolderNotEmpty = errors.New("folder is not empty")

// ErrPasswordNotFound indicates the requested password entry does not exist in the vault.
var ErrPasswordNotFound = errors.New("password not found in vault")

// ErrVPNProfileNotFound indicates the requested VPN profile does not exist.
var ErrVPNProfileNotFound = errors.New("VPN profile not found")

// ErrFolderNotFound indicates the requested folder does not exist.
var ErrFolderNotFound = errors.New("folder not found")

// ErrCircularFolder indicates a folder move would create a circular parent chain.
var ErrCircularFolder = errors.New("circular folder hierarchy detected")

// ErrConnectionNotFound indicates no connection exists with the given ID.
var ErrConnectionNotFound = errors.New("connection not found")

// ErrAuditLogWrite indicates an audit log entry could not be persisted.
var ErrAuditLogWrite = errors.New("failed to write audit log entry")
