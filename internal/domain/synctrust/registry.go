// Package synctrust is the client-side meaning of sync records (ADR-023): which record types exist
// and how they are named, how a local vault maps onto records and back, which settings travel,
// which merge conflicts a person must resolve, and which incoming changes may weaken this device
// and therefore wait for the master password.
//
// It sits above vaultsync (the protocol) and internal/domain (the vault's types), and below the use
// cases. Nothing in the server imports it: the server never sees a record type.
package synctrust

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// RecordType names what a record holds. It travels inside the sealed body only.
type RecordType string

const (
	TypeVaultMeta    RecordType = "vault-meta"
	TypeFolder       RecordType = "folder"
	TypeConnection   RecordType = "connection"
	TypeIdentity     RecordType = "identity"
	TypePassword     RecordType = "password"
	TypePluginSecret RecordType = "plugin-secret"
	TypeKnownHost    RecordType = "known-host"
	TypePeerTrust    RecordType = "peer-trust"
	TypeConflict     RecordType = "conflict"
	TypeContact      RecordType = "contact"
	// TypeTeam is a team vault the user joined. Only joined teams sync into the local vault.
	TypeTeam RecordType = "team"
	// TypeCredentialBinding ties a team connection user to this member's own credential.
	TypeCredentialBinding RecordType = "credential-binding" // #nosec G101 -- a record type name, not a credential
	// TypePluginInstall is a plugin installed on one of the user's devices, by reference.
	TypePluginInstall RecordType = "plugin-install"
	// TypePluginData is one data file a plugin declared for sync.
	TypePluginData RecordType = "plugin-data"

	settingsPrefix = "settings."
)

// settingsSections are the settings that travel, one record each; the field-level split is in
// settings.go. Device-local settings have no section and never leave the device.
var settingsSections = []string{
	"lockout", "terminal", "appearance", "ping", "transfer", "sessionHotkeys",
	"localTerminal", "terminalTools", "plugins", "embed", "updates", "reconnect",
}

// SettingsType is the record type of one settings section.
func SettingsType(section string) RecordType { return RecordType(settingsPrefix + section) }

// schemaV1 lists every type this build reads and writes, all at schema 1. Any change to a type's
// data bumps its number; older builds then show such records read-only (vaultsync.RecordAccess).
var schemaV1 = []RecordType{
	TypeVaultMeta, TypeFolder, TypeConnection, TypeIdentity, TypePassword, TypePluginSecret,
	TypeKnownHost, TypePeerTrust, TypeConflict, TypeContact, TypeCredentialBinding, TypePluginInstall, TypePluginData,
	TypeTeam,
}

// KnownSchemas is the registry vaultsync.RecordAccess consults.
func KnownSchemas() map[string]uint16 {
	out := make(map[string]uint16, len(schemaV1)+len(settingsSections))
	for _, t := range schemaV1 {
		out[string(t)] = 1
	}
	for _, s := range settingsSections {
		out[string(SettingsType(s))] = 1
	}
	return out
}

// Body is a record's sealed content: its type, the type's schema version, and the type's data.
type Body struct {
	Type   RecordType      `json:"type"`
	Schema uint16          `json:"schema"`
	Data   json.RawMessage `json:"data"`
}

var (
	// ErrBody is a record body that does not decode into exactly a Body.
	ErrBody = errors.New("malformed record body")
	// ErrNamingKey is a naming key of the wrong size.
	ErrNamingKey = errors.New("record naming key must be 32 bytes")
)

// EncodeBody serialises a body for sealing.
func EncodeBody(b Body) ([]byte, error) { return json.Marshal(b) }

// DecodeBody parses an opened record strictly: one JSON object, no unknown fields, a type, a
// schema and object data. Bodies come from other devices and are untrusted input.
func DecodeBody(raw []byte) (Body, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var b Body
	if err := dec.Decode(&b); err != nil {
		return Body{}, fmt.Errorf("%w: %w", ErrBody, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Body{}, fmt.Errorf("%w: data after the body", ErrBody)
	}
	if b.Type == "" || b.Schema == 0 || len(b.Data) == 0 || b.Data[0] != '{' {
		return Body{}, fmt.Errorf("%w: incomplete", ErrBody)
	}
	return b, nil
}

// NamingKeySize is the length of a vault's record naming key.
const NamingKeySize = 32

const (
	labelRecordName = "xqs-sync/v1/record-name"
	labelVaultMeta  = "xqs-sync/v1/vault-meta"
)

// RecordID names the record for one natural key of one type. It is keyed with the vault's naming
// key, so every device derives the same id for a settings section or a host - no duplicates - while
// the server, which never holds the naming key, cannot test guesses such as a host name against it.
func RecordID(namingKey []byte, t RecordType, naturalKey string) (vaultsync.ID, error) {
	if len(namingKey) != NamingKeySize {
		return vaultsync.ID{}, ErrNamingKey
	}
	msg, err := vaultsync.NewCanon(labelRecordName).String(string(t)).String(naturalKey).Encoded()
	if err != nil {
		return vaultsync.ID{}, err
	}
	mac := hmac.New(sha256.New, namingKey)
	mac.Write(msg)
	var id vaultsync.ID
	copy(id[:], mac.Sum(nil))
	if id.IsZero() {
		return vaultsync.ID{}, vaultsync.ErrZeroID
	}
	return id, nil
}

// VaultMetaID is the fixed id of a vault's vault-meta record, which holds the naming key. It is
// derived from the vault id alone so a new device can find it before it knows the naming key.
func VaultMetaID(vault vaultsync.ID) vaultsync.ID {
	sum := sha256.Sum256(append([]byte(labelVaultMeta), vault[:]...))
	var id vaultsync.ID
	copy(id[:], sum[:])
	return id
}

// IsSettingsType reports whether t is a known settings section.
func IsSettingsType(t RecordType) (string, bool) {
	s, ok := bytes.CutPrefix([]byte(t), []byte(settingsPrefix))
	return string(s), ok && slices.Contains(settingsSections, string(s))
}
