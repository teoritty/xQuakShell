package vaultsync

import (
	"fmt"
	"unicode/utf8"
)

// DeviceOp is the kind of a device log entry.
type DeviceOp uint8

const (
	// OpAddDevice certifies a new device's signing key under the account identity key.
	OpAddDevice DeviceOp = 1
	// OpRevokeDevice ends a device's ability to authenticate; it must be followed by OpSuccession.
	OpRevokeDevice DeviceOp = 2
	// OpSuccession rotates the account identity and encryption keys with continuity.
	OpSuccession DeviceOp = 3
)

const (
	maxDeviceNameBytes     = 64
	maxDevicePlatformBytes = 32
)

// DeviceCert is what an account says about one of its devices.
type DeviceCert struct {
	Device           ID
	SigningKey       []byte
	Name             string
	Platform         string
	CreatedUnixMilli uint64
}

// DeviceEntry is one link of an account's device log (spec section 5.4). Sig is by the account
// identity key in force at this position; SuccessorSig, on a succession only, is by the new key
// over the same bytes, proving the new key's holder took part.
type DeviceEntry struct {
	Account          ID
	Seq              uint64
	Prev             [32]byte
	Op               DeviceOp
	Cert             *DeviceCert
	Revoked          ID
	NewSuite         SuiteID
	NewIdentityKey   []byte
	NewEncryptionKey []byte
	SigAlg           SigAlg
	Sig              []byte
	SuccessorSig     []byte
}

// SigningBytes encodes every field except the two signatures. Absent payloads encode as empty
// fields, so each op signs one fixed layout.
func (e *DeviceEntry) SigningBytes() ([]byte, error) {
	c := NewCanon(LabelDeviceSig).ID(e.Account).U64(e.Seq).Hash(e.Prev).U16(uint16(e.Op))
	c.Bool(e.Cert != nil)
	if e.Cert != nil {
		c.ID(e.Cert.Device).Bytes(e.Cert.SigningKey).String(e.Cert.Name).String(e.Cert.Platform).U64(e.Cert.CreatedUnixMilli)
	}
	c.ID(e.Revoked).U16(uint16(e.NewSuite)).Bytes(e.NewIdentityKey).Bytes(e.NewEncryptionKey).U16(uint16(e.SigAlg))
	return c.Encoded()
}

func (e *DeviceEntry) hash(msg []byte) ([32]byte, error) {
	return NewCanon(LabelDeviceHash).Bytes(msg).Bytes(e.Sig).Bytes(e.SuccessorSig).Sum()
}

func validateCert(c *DeviceCert, signingKeySize int) error {
	switch {
	case c == nil:
		return fmt.Errorf("%w: add-device without a certificate", ErrDeviceLog)
	case c.Device.IsZero():
		return fmt.Errorf("%w: zero device id", ErrDeviceLog)
	case len(c.SigningKey) != signingKeySize:
		return fmt.Errorf("%w: signing key size", ErrDeviceLog)
	case len(c.Name) > maxDeviceNameBytes || !utf8.ValidString(c.Name):
		return fmt.Errorf("%w: device name", ErrDeviceLog)
	case len(c.Platform) > maxDevicePlatformBytes || !utf8.ValidString(c.Platform):
		return fmt.Errorf("%w: device platform", ErrDeviceLog)
	}
	return nil
}
