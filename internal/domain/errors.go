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

// ErrVaultNotFound indicates no vault file exists yet; the master password must be created first.
var ErrVaultNotFound = errors.New("vault not found")

// ErrVaultAlreadyExists indicates a vault file is already present and must not be overwritten.
var ErrVaultAlreadyExists = errors.New("vault already exists")

// ErrMasterPasswordTooShort indicates the proposed master password is below the minimum length policy.
var ErrMasterPasswordTooShort = errors.New("master password is too short")

// ErrVaultVersionTooNew indicates the vault was written by a newer build than this one. The two
// directions are separate errors because only one of them is recoverable here: a newer file must
// never be touched — this build would have to guess at fields it does not know, and writing back
// what it understood would silently drop the rest — while an older one is a migration this build
// could perform. Collapsing both into one error costs the user the difference between "install the
// newer version again" and "let it upgrade your data".
var ErrVaultVersionTooNew = errors.New("vault was written by a newer version of the application")

// ErrVaultVersionTooOld indicates the vault predates this build's schema and needs migrating.
var ErrVaultVersionTooOld = errors.New("vault needs migrating to the current schema")

// ErrNoStableRelease indicates the project has no published stable release to compare against —
// only drafts or pre-releases. It is not a failure the user needs to see: it means there is nothing
// to upgrade to, which is indistinguishable from being up to date.
var ErrNoStableRelease = errors.New("no stable release published")

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

// ErrFolderNotFound indicates the requested folder does not exist.
var ErrFolderNotFound = errors.New("folder not found")

// ErrCircularFolder indicates a folder move would create a circular parent chain.
var ErrCircularFolder = errors.New("circular folder hierarchy detected")

// ErrConnectionNotFound indicates no connection exists with the given ID.
var ErrConnectionNotFound = errors.New("connection not found")

// ErrForwardRuleNotFound indicates the requested forward rule does not exist on the connection.
var ErrForwardRuleNotFound = errors.New("forward rule not found")

// ErrAuditLogWrite indicates an audit log entry could not be persisted.
var ErrAuditLogWrite = errors.New("failed to write audit log entry")
