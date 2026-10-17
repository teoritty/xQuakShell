package synctrust

import (
	"encoding/json"
	"fmt"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

type vaultMetaData struct {
	NamingKey []byte `json:"namingKey"`
	// Name is a team vault's name as its members see it; a personal vault has none.
	Name string `json:"name,omitempty"`
}

// NewVaultMetaBody is the body of a vault's vault-meta record. It is written once, in the vault's
// first commit, and never changed: every record id in the vault is derived from this key, so a new
// key would orphan them all. The engine treats a second, different vault-meta as an integrity
// failure.
func NewVaultMetaBody(namingKey []byte) (Body, error) {
	return NewTeamMetaBody(namingKey, "")
}

// NewTeamMetaBody is a team vault's vault-meta body, carrying the name its members see.
func NewTeamMetaBody(namingKey []byte, name string) (Body, error) {
	if len(namingKey) != NamingKeySize {
		return Body{}, ErrNamingKey
	}
	data, err := json.Marshal(vaultMetaData{NamingKey: namingKey, Name: name})
	if err != nil {
		return Body{}, fmt.Errorf("encode vault-meta: %w", err)
	}
	return Body{Type: TypeVaultMeta, Schema: 1, Data: data}, nil
}

// VaultMetaName reads a team vault's name out of a vault-meta body; empty when it carries none.
func VaultMetaName(b Body) string {
	v, err := decodeData[vaultMetaData](b.Data)
	if err != nil {
		return ""
	}
	return v.Name
}

// VaultMetaNamingKey reads the naming key out of a vault-meta body.
func VaultMetaNamingKey(b Body) ([]byte, error) {
	if b.Type != TypeVaultMeta {
		return nil, fmt.Errorf("%w: %s is not vault-meta", ErrRecordData, b.Type)
	}
	v, err := decodeData[vaultMetaData](b.Data)
	if err != nil {
		return nil, err
	}
	if len(v.NamingKey) != NamingKeySize {
		return nil, ErrNamingKey
	}
	return slices.Clone(v.NamingKey), nil
}

// KeyRing is this device's view of its account's keys while the vault is unlocked. Implementations
// keep the secrets; the engine only asks for what one operation needs.
type KeyRing interface {
	// Account is the signed-in account and this device's id in it.
	Account() vaultsync.ID
	Device() vaultsync.ID
	// AccountKeys are the account's genesis public keys and their suite, the root the device log is
	// verified from.
	AccountKeys() (suite vaultsync.SuiteID, identity, encryption []byte)
	// CurrentKeys are the public keys of the seeds this device holds. They must be the keys the
	// replayed device log ends with; otherwise another device rotated them.
	CurrentKeys() (identity, encryption []byte)
	// VaultKey unwraps the key of one vault epoch from that vault's membership log.
	VaultKey(vault vaultsync.ID, epoch uint32, ops []vaultsync.MemberOp) ([]byte, error)
	// Sign signs with this device's signing key.
	Sign(msg []byte) ([]byte, error)
}

// RecordCipher seals and opens record bodies; synccrypto implements it.
type RecordCipher interface {
	// Open decrypts a live envelope's body, authenticated to every coordinate of the envelope.
	Open(vaultKey []byte, e *vaultsync.Envelope) ([]byte, error)
	// Seal fills e.WrappedDEK and e.Body for plaintext under a fresh data key.
	Seal(vaultKey []byte, e *vaultsync.Envelope, plaintext []byte) error
}
