package syncwire

import (
	"bytes"
	"fmt"

	"xquakshell/internal/domain/vaultsync"
)

// DeviceCert is the wire form of vaultsync.DeviceCert.
type DeviceCert struct {
	Device           string `json:"device"`
	SigningKey       []byte `json:"signingKey"`
	Name             string `json:"name"`
	Platform         string `json:"platform"`
	CreatedUnixMilli uint64 `json:"createdUnixMilli"`
}

// DeviceEntry is the wire form of vaultsync.DeviceEntry.
type DeviceEntry struct {
	Account          string      `json:"account"`
	Seq              uint64      `json:"seq"`
	Prev             []byte      `json:"prev"`
	Op               uint8       `json:"op"`
	Cert             *DeviceCert `json:"cert,omitempty"`
	Revoked          string      `json:"revoked,omitempty"`
	NewSuite         uint16      `json:"newSuite,omitempty"`
	NewIdentityKey   []byte      `json:"newIdentityKey,omitempty"`
	NewEncryptionKey []byte      `json:"newEncryptionKey,omitempty"`
	SigAlg           uint16      `json:"sigAlg"`
	Sig              []byte      `json:"sig"`
	SuccessorSig     []byte      `json:"successorSig,omitempty"`
}

// DeviceEntryFrom converts a domain device log entry for the wire.
func DeviceEntryFrom(e *vaultsync.DeviceEntry) DeviceEntry {
	d := DeviceEntry{
		Account: e.Account.String(), Seq: e.Seq, Prev: bytes.Clone(e.Prev[:]), Op: uint8(e.Op),
		Revoked: optionalID(e.Revoked), NewSuite: uint16(e.NewSuite), NewIdentityKey: bytes.Clone(e.NewIdentityKey),
		NewEncryptionKey: bytes.Clone(e.NewEncryptionKey), SigAlg: uint16(e.SigAlg), Sig: bytes.Clone(e.Sig),
		SuccessorSig: bytes.Clone(e.SuccessorSig),
	}
	if e.Cert != nil {
		d.Cert = &DeviceCert{Device: e.Cert.Device.String(), SigningKey: bytes.Clone(e.Cert.SigningKey),
			Name: e.Cert.Name, Platform: e.Cert.Platform, CreatedUnixMilli: e.Cert.CreatedUnixMilli}
	}
	return d
}

// ToDomain converts and checks identifiers and hash lengths.
func (d DeviceEntry) ToDomain() (vaultsync.DeviceEntry, error) {
	account, err := parseID(d.Account)
	if err != nil {
		return vaultsync.DeviceEntry{}, err
	}
	prev, err := parseHash(d.Prev)
	if err != nil {
		return vaultsync.DeviceEntry{}, err
	}
	revoked, err := parseOptionalID(d.Revoked)
	if err != nil {
		return vaultsync.DeviceEntry{}, err
	}
	e := vaultsync.DeviceEntry{
		Account: account, Seq: d.Seq, Prev: prev, Op: vaultsync.DeviceOp(d.Op), Revoked: revoked,
		NewSuite: vaultsync.SuiteID(d.NewSuite), NewIdentityKey: d.NewIdentityKey, NewEncryptionKey: d.NewEncryptionKey,
		SigAlg: vaultsync.SigAlg(d.SigAlg), Sig: d.Sig, SuccessorSig: d.SuccessorSig,
	}
	if d.Cert != nil {
		device, err := parseID(d.Cert.Device)
		if err != nil {
			return vaultsync.DeviceEntry{}, fmt.Errorf("certificate: %w", err)
		}
		e.Cert = &vaultsync.DeviceCert{Device: device, SigningKey: d.Cert.SigningKey, Name: d.Cert.Name,
			Platform: d.Cert.Platform, CreatedUnixMilli: d.Cert.CreatedUnixMilli}
	}
	return e, nil
}

// KeyWrap is the wire form of vaultsync.KeyWrap.
type KeyWrap struct {
	Member     string `json:"member"`
	Ciphertext []byte `json:"ciphertext"`
}

// MemberOp is the wire form of vaultsync.MemberOp.
type MemberOp struct {
	Vault           string    `json:"vault"`
	Seq             uint64    `json:"seq"`
	Prev            []byte    `json:"prev"`
	Kind            uint8     `json:"kind"`
	Author          string    `json:"author"`
	AuthorDevice    string    `json:"authorDevice"`
	Subject         string    `json:"subject,omitempty"`
	Role            uint8     `json:"role,omitempty"`
	Verified        bool      `json:"verified,omitempty"`
	Fingerprint     []byte    `json:"fingerprint"`
	Suite           uint16    `json:"suite"`
	Epoch           uint32    `json:"epoch"`
	Wraps           []KeyWrap `json:"wraps,omitempty"`
	RevokedDevices  []string  `json:"revokedDevices,omitempty"`
	RetentionDays   uint16    `json:"retentionDays,omitempty"`
	RequireVerified bool      `json:"requireVerified,omitempty"`
	TimeUnixMilli   uint64    `json:"timeUnixMilli"`
	SigAlg          uint16    `json:"sigAlg"`
	Sig             []byte    `json:"sig"`
}

// MemberOpFrom converts a domain membership operation for the wire.
func MemberOpFrom(op *vaultsync.MemberOp) MemberOp {
	d := MemberOp{
		Vault: op.Vault.String(), Seq: op.Seq, Prev: bytes.Clone(op.Prev[:]), Kind: uint8(op.Kind),
		Author: op.Author.String(), AuthorDevice: op.AuthorDevice.String(), Subject: optionalID(op.Subject),
		Role: uint8(op.Role), Verified: op.Verified, Fingerprint: bytes.Clone(op.Fingerprint[:]), Suite: uint16(op.Suite),
		Epoch: op.Epoch, RevokedDevices: idStrings(op.RevokedDevices), RetentionDays: op.RetentionDays,
		RequireVerified: op.RequireVerified, TimeUnixMilli: op.TimeUnixMilli, SigAlg: uint16(op.SigAlg), Sig: bytes.Clone(op.Sig),
	}
	for _, w := range op.Wraps {
		d.Wraps = append(d.Wraps, KeyWrap{Member: w.Member.String(), Ciphertext: bytes.Clone(w.Ciphertext)})
	}
	return d
}

// ToDomain converts and checks identifiers and hash lengths.
func (d MemberOp) ToDomain() (vaultsync.MemberOp, error) {
	var op vaultsync.MemberOp
	var err error
	if op.Vault, err = parseID(d.Vault); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.Author, err = parseID(d.Author); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.AuthorDevice, err = parseID(d.AuthorDevice); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.Subject, err = parseOptionalID(d.Subject); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.Prev, err = parseHash(d.Prev); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.Fingerprint, err = parseHash(d.Fingerprint); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.RevokedDevices, err = parseIDs(d.RevokedDevices); err != nil {
		return vaultsync.MemberOp{}, err
	}
	if op.Wraps, err = parseWraps(d.Wraps); err != nil {
		return vaultsync.MemberOp{}, err
	}
	op.Seq, op.Kind, op.Role, op.Verified = d.Seq, vaultsync.MemberOpKind(d.Kind), vaultsync.Role(d.Role), d.Verified
	op.Suite, op.Epoch, op.RetentionDays, op.RequireVerified = vaultsync.SuiteID(d.Suite), d.Epoch, d.RetentionDays, d.RequireVerified
	op.TimeUnixMilli, op.SigAlg, op.Sig = d.TimeUnixMilli, vaultsync.SigAlg(d.SigAlg), d.Sig
	return op, nil
}

func parseWraps(ws []KeyWrap) ([]vaultsync.KeyWrap, error) {
	if ws == nil {
		return nil, nil
	}
	out := make([]vaultsync.KeyWrap, len(ws))
	for i, w := range ws {
		member, err := parseID(w.Member)
		if err != nil {
			return nil, err
		}
		out[i] = vaultsync.KeyWrap{Member: member, Ciphertext: w.Ciphertext}
	}
	return out, nil
}
