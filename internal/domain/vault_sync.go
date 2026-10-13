package domain

import (
	"encoding/json"
	"strconv"
	"strings"
)

// VaultSync is this device's sync state inside the encrypted vault (ADR-023). The field is additive
// and omitempty like PeerTrust, so the vault schema does not move: an older build that drops it loses
// only what is listed here and has to sign in to sync again.
type VaultSync struct {
	// Conflicts are credential fields two devices set to different values. The vault keeps the value
	// the server accepted first; the other one waits here, encrypted with the rest of the vault and
	// synced to every device, until a person picks one.
	Conflicts []SyncConflict `json:"conflicts,omitempty"`
	// Contacts are the accounts whose key this user has seen, synced to every device of theirs: the
	// anchor a team vault's members are verified from (spec §6.6).
	Contacts []SyncContact `json:"contacts,omitempty"`
	// Bindings tie team connection users to this user's own credentials (spec §6.7).
	Bindings []CredentialBinding `json:"bindings,omitempty"`
	// Plugins are the plugins installed on the user's devices, by reference (spec §6.4).
	Plugins []SyncPlugin `json:"plugins,omitempty"`
	// PluginFiles mirror the data files plugins declared for sync (spec §6.5). The vault is the
	// synced copy; the plugin's data directory receives it only while the plugin is stopped.
	PluginFiles []SyncPluginFile `json:"pluginFiles,omitempty"`
	// Account holds this device's credentials for its sync account; nil until sync is enabled. It
	// is never synced as a record: these are the keys every record is protected with.
	Account *SyncAccount `json:"account,omitempty"`
}

// SyncAccount is what a device needs to sync. The seeds are secrets and live only here, inside the
// vault the master password encrypts; the server holds them only sealed under the sync password.
type SyncAccount struct {
	ServerURL string `json:"serverUrl"`
	// ServerIdentity is the server identity key the user confirmed by fingerprint.
	ServerIdentity []byte `json:"serverIdentity"`
	Username       string `json:"username"`
	Account        []byte `json:"account"`
	Device         []byte `json:"device"`
	// DeviceSeed is this device's signing key; it never leaves the device.
	DeviceSeed []byte `json:"deviceSeed"`
	// IdentitySeed and EncryptionSeed are the account keys from the bundle.
	IdentitySeed   []byte `json:"identitySeed"`
	EncryptionSeed []byte `json:"encryptionSeed"`
	EncryptionKey  []byte `json:"encryptionKey"`
	// GenesisIdentityKey and GenesisEncryptionKey are the account keys the device log starts from.
	// After a succession they differ from the current ones, and the log is still replayed from them.
	GenesisIdentityKey   []byte `json:"genesisIdentityKey"`
	GenesisEncryptionKey []byte `json:"genesisEncryptionKey"`
	// Pending holds new account keys between generating them and the device log accepting their
	// succession, so a crash in between cannot lose the keys the log now names.
	Pending *SyncPendingKeys `json:"pending,omitempty"`
	// RekeyPending marks a succession whose new bundle has not reached the server yet: until it
	// does, no new device can sign in, and the user is asked to finish with the sync password.
	RekeyPending bool `json:"rekeyPending,omitempty"`
	// JoinPending marks a login that found data on both sides and is waiting for the user to choose
	// merge or replace; until then this device does not sync, so the backup the join takes first is
	// never skipped by a restart.
	JoinPending bool `json:"joinPending,omitempty"`
}

// SyncPendingKeys are account keys generated for a succession not yet confirmed.
type SyncPendingKeys struct {
	IdentitySeed   []byte `json:"identitySeed"`
	EncryptionSeed []byte `json:"encryptionSeed"`
	EncryptionKey  []byte `json:"encryptionKey"`
}

// SyncConflict is one unresolved credential conflict.
type SyncConflict struct {
	// ID is ConflictID(RecordType, ItemKey, Path); a second conflict on the same field replaces the
	// first rather than piling up beside it.
	ID          string          `json:"id"`
	RecordType  string          `json:"recordType"`
	ItemKey     string          `json:"itemKey"`
	Path        []string        `json:"path"`
	Local       json.RawMessage `json:"local,omitempty"`
	Remote      json.RawMessage `json:"remote,omitempty"`
	AtUnixMilli int64           `json:"at"`
	// VaultID is the team vault of the item in conflict; empty is the personal vault.
	VaultID string `json:"vaultId,omitempty"`
}

// SyncContact is one account whose genesis identity key this user trusts. Verified is set when the
// user compared its fingerprint out of band; otherwise it was trusted on first use, which the UI
// marks.
type SyncContact struct {
	// Account and IdentityDigest are hex: the account id and vaultsync.IdentityDigest of its key.
	Account        string `json:"account"`
	IdentityDigest string `json:"identityDigest"`
	Username       string `json:"username,omitempty"`
	Verified       bool   `json:"verified"`
}

// ConflictID names the conflict on one field of one item. Each part is quoted, so no separator can
// be forged: item keys may contain any character, including the newline peer-trust keys use.
func ConflictID(recordType, itemKey string, path []string) string {
	parts := make([]string, 0, len(path)+2)
	for _, p := range append([]string{recordType, itemKey}, path...) {
		parts = append(parts, strconv.Quote(p))
	}
	return strings.Join(parts, ",")
}
