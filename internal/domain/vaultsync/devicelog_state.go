package vaultsync

import (
	"bytes"
	"fmt"
	"slices"
)

type deviceStatus struct {
	cert    DeviceCert
	revoked bool
}

// DeviceLogState is an account's device log replayed from genesis. The starting identity key comes
// from outside the log - from the password-sealed bundle for the account's own devices, or from a
// verified fingerprint for a team co-member - and everything after it is checked against the keys
// the log itself installs.
type DeviceLogState struct {
	account           ID
	params            SuiteParams
	identityKey       []byte
	encryptionKey     []byte
	superseded        [][]byte
	devices           map[ID]deviceStatus
	seq               uint64
	hash              [32]byte
	pendingSuccession bool
}

// NewDeviceLogState starts a log at its genesis position for the given account keys.
func NewDeviceLogState(account ID, suite SuiteID, identityKey, encryptionKey []byte) (*DeviceLogState, error) {
	params, err := LookupSuite(suite, MinAcceptedSuite)
	if err != nil {
		return nil, err
	}
	if len(identityKey) != params.SigningKeySize || len(encryptionKey) != params.EncryptionKeySize {
		return nil, fmt.Errorf("%w: account key size", ErrDeviceLog)
	}
	// The genesis position commits to the account keys. The first entry is signed by the identity
	// key and chains from here, so it vouches for the encryption key too - the key team invites wrap
	// vault keys to, which nothing else signed would bind.
	genesis, err := NewCanon(LabelDeviceHash).ID(account).U16(uint16(suite)).Bytes(identityKey).Bytes(encryptionKey).Sum()
	if err != nil {
		return nil, err
	}
	return &DeviceLogState{
		account: account, params: params, identityKey: bytes.Clone(identityKey),
		encryptionKey: bytes.Clone(encryptionKey), devices: map[ID]deviceStatus{}, hash: genesis,
	}, nil
}

// Apply verifies one entry and, only if every check passes, applies it. A rejected entry leaves
// the state exactly as it was: the chain hash is computed before anything changes, and each op
// validates completely before its first assignment.
func (s *DeviceLogState) Apply(e *DeviceEntry) error {
	if e.Account != s.account || e.Seq != s.seq+1 || e.Prev != s.hash {
		return fmt.Errorf("%w: entry does not extend the log at seq %d", ErrDeviceLog, s.seq)
	}
	msg, err := e.SigningBytes()
	if err != nil {
		return err
	}
	if err := s.checkSignature(e, msg); err != nil {
		return err
	}
	// Further revocations may precede the one succession that answers them all; nothing else may.
	if s.pendingSuccession && e.Op != OpSuccession && e.Op != OpRevokeDevice {
		return ErrSuccessionRequired
	}
	next, err := e.hash(msg)
	if err != nil {
		return err
	}
	if err := s.applyOp(e, msg); err != nil {
		return err
	}
	s.seq, s.hash = e.Seq, next
	return nil
}

// checkSignature reports a signature by a superseded identity key as that, distinctly: it is
// exactly what a revoked device holding the old key would produce.
func (s *DeviceLogState) checkSignature(e *DeviceEntry, msg []byte) error {
	err := VerifySignature(e.SigAlg, s.identityKey, msg, e.Sig)
	if err == nil {
		return nil
	}
	for _, old := range s.superseded {
		if VerifySignature(e.SigAlg, old, msg, e.Sig) == nil {
			return ErrSupersededKey
		}
	}
	return err
}

func (s *DeviceLogState) applyOp(e *DeviceEntry, msg []byte) error {
	switch e.Op {
	case OpAddDevice:
		return s.addDevice(e.Cert)
	case OpRevokeDevice:
		return s.revokeDevice(e.Revoked)
	case OpSuccession:
		return s.succeed(e, msg)
	default:
		return fmt.Errorf("%w: unknown op %d", ErrDeviceLog, e.Op)
	}
}

func (s *DeviceLogState) addDevice(c *DeviceCert) error {
	if err := validateCert(c, s.params.SigningKeySize); err != nil {
		return err
	}
	if _, known := s.devices[c.Device]; known {
		return fmt.Errorf("%w: device %s already certified", ErrDeviceLog, c.Device)
	}
	cert := *c
	cert.SigningKey = bytes.Clone(c.SigningKey)
	s.devices[c.Device] = deviceStatus{cert: cert}
	return nil
}

func (s *DeviceLogState) revokeDevice(id ID) error {
	st, known := s.devices[id]
	if !known || st.revoked {
		return fmt.Errorf("%w: cannot revoke device %s", ErrDeviceLog, id)
	}
	st.revoked = true
	s.devices[id] = st
	s.pendingSuccession = true
	return nil
}

func (s *DeviceLogState) succeed(e *DeviceEntry, msg []byte) error {
	params, err := LookupSuite(e.NewSuite, MinAcceptedSuite)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDeviceLog, err)
	}
	if len(e.NewIdentityKey) != params.SigningKeySize || len(e.NewEncryptionKey) != params.EncryptionKeySize {
		return fmt.Errorf("%w: successor key size", ErrDeviceLog)
	}
	if err := VerifySignature(e.SigAlg, e.NewIdentityKey, msg, e.SuccessorSig); err != nil {
		return err
	}
	s.superseded = append(s.superseded, s.identityKey)
	s.identityKey = bytes.Clone(e.NewIdentityKey)
	s.encryptionKey = bytes.Clone(e.NewEncryptionKey)
	s.params = params
	s.pendingSuccession = false
	return nil
}

// Seq is the position of the last applied entry.
func (s *DeviceLogState) Seq() uint64 { return s.seq }

// Hash is the chain hash of the last applied entry, which the next entry's Prev must equal.
func (s *DeviceLogState) Hash() [32]byte { return s.hash }

// IdentityKey is the account identity public key currently in force.
func (s *DeviceLogState) IdentityKey() []byte { return bytes.Clone(s.identityKey) }

// EncryptionKey is the account encryption public key that vault keys are wrapped to.
func (s *DeviceLogState) EncryptionKey() []byte { return bytes.Clone(s.encryptionKey) }

// DeviceKey returns a certified device's signing key whether or not the device was revoked since.
// Revocation does not reach back: a commit the device made while it was trusted stays valid, and a
// new device verifying a snapshot must still accept it. When a vault stops accepting a device is
// decided by that vault's membership log.
func (s *DeviceLogState) DeviceKey(id ID) ([]byte, error) {
	st, known := s.devices[id]
	if !known {
		return nil, fmt.Errorf("%w: device %s not certified", ErrDeviceLog, id)
	}
	return bytes.Clone(st.cert.SigningKey), nil
}

// IsRevoked reports whether the log has revoked this device; the server uses it to refuse logins.
func (s *DeviceLogState) IsRevoked(id ID) bool { return s.devices[id].revoked }

// PendingSuccession reports a revocation not yet followed by a succession. A batch of entries that
// ends in this state is refused as a whole by its caller.
func (s *DeviceLogState) PendingSuccession() bool { return s.pendingSuccession }

// Devices lists certified device ids in a stable order.
func (s *DeviceLogState) Devices() []ID {
	ids := make([]ID, 0, len(s.devices))
	for id := range s.devices {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b ID) int { return bytes.Compare(a[:], b[:]) })
	return ids
}

// Certificate returns a copy of a certified device's certificate - its name, platform and key - for
// showing the account's devices; revoked devices keep theirs.
func (s *DeviceLogState) Certificate(id ID) (DeviceCert, bool) {
	st, known := s.devices[id]
	if !known {
		return DeviceCert{}, false
	}
	c := st.cert
	c.SigningKey = bytes.Clone(st.cert.SigningKey)
	return c, true
}
