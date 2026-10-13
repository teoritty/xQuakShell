package vaultsync_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

var account = vaultsync.ID{0xac}

func aekBytes(seed byte) []byte { return bytes.Repeat([]byte{seed}, 1216) }

func newLog(t *testing.T, aik ed25519.PrivateKey) *vaultsync.DeviceLogState {
	t.Helper()
	s, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, aik.Public().(ed25519.PublicKey), aekBytes(1))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func signEntry(t *testing.T, s *vaultsync.DeviceLogState, aik ed25519.PrivateKey, e *vaultsync.DeviceEntry) *vaultsync.DeviceEntry {
	t.Helper()
	e.Account, e.Seq, e.Prev, e.SigAlg = account, s.Seq()+1, s.Hash(), vaultsync.SigEd25519
	msg, err := e.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	e.Sig = ed25519.Sign(aik, msg)
	return e
}

func addDevice(id byte, dsk ed25519.PrivateKey) *vaultsync.DeviceEntry {
	return &vaultsync.DeviceEntry{Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{
		Device: vaultsync.ID{id}, SigningKey: dsk.Public().(ed25519.PublicKey), Name: "laptop", Platform: "linux", CreatedUnixMilli: 1,
	}}
}

func succession(t *testing.T, s *vaultsync.DeviceLogState, oldAIK, newAIK ed25519.PrivateKey) *vaultsync.DeviceEntry {
	t.Helper()
	e := signEntry(t, s, oldAIK, &vaultsync.DeviceEntry{
		Op: vaultsync.OpSuccession, NewSuite: vaultsync.Suite1,
		NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: aekBytes(2),
	})
	msg, _ := e.SigningBytes()
	e.SuccessorSig = ed25519.Sign(newAIK, msg)
	return e
}

func TestDeviceLogAddsDevice(t *testing.T) {
	aik, dsk := testKey(10), testKey(20)
	s := newLog(t, aik)
	if err := s.Apply(signEntry(t, s, aik, addDevice(1, dsk))); err != nil {
		t.Fatal(err)
	}
	key, err := s.DeviceKey(vaultsync.ID{1})
	if err != nil || !bytes.Equal(key, dsk.Public().(ed25519.PublicKey)) {
		t.Fatalf("DeviceKey = %x, %v; want the certified signing key", key, err)
	}
	if got := s.Devices(); len(got) != 1 || got[0] != (vaultsync.ID{1}) {
		t.Fatalf("Devices = %v, want [device 1]", got)
	}
}

func TestDeviceLogRejectsPositionAndSignatureErrors(t *testing.T) {
	aik := testKey(10)
	cases := map[string]func(*vaultsync.DeviceEntry){
		"skipped seq":   func(e *vaultsync.DeviceEntry) { e.Seq++ },
		"wrong prev":    func(e *vaultsync.DeviceEntry) { e.Prev[0] ^= 1 },
		"wrong account": func(e *vaultsync.DeviceEntry) { e.Account[0] ^= 1 },
		"bad signature": func(e *vaultsync.DeviceEntry) { e.Sig[0] ^= 1 },
	}
	for name, mutate := range cases {
		s := newLog(t, aik)
		e := signEntry(t, s, aik, addDevice(1, testKey(20)))
		mutate(e)
		if err := s.Apply(e); !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Errorf("%s: err = %v, want an integrity error", name, err)
		}
		if s.Seq() != 0 {
			t.Errorf("%s: a rejected entry advanced the log", name)
		}
	}
}

func TestDeviceLogRevocationRequiresSuccession(t *testing.T) {
	aik := testKey(10)
	s := newLog(t, aik)
	_ = s.Apply(signEntry(t, s, aik, addDevice(1, testKey(20))))
	_ = s.Apply(signEntry(t, s, aik, addDevice(2, testKey(21))))
	if err := s.Apply(signEntry(t, s, aik, &vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{2}})); err != nil {
		t.Fatal(err)
	}
	if !s.PendingSuccession() {
		t.Fatal("revocation did not mark a succession as pending")
	}
	if err := s.Apply(signEntry(t, s, aik, addDevice(3, testKey(22)))); !errors.Is(err, vaultsync.ErrSuccessionRequired) {
		t.Fatalf("err = %v, want ErrSuccessionRequired; a revoked device must not keep the old AIK useful", err)
	}
}

func TestDeviceLogSuccessionSupersedesOldKey(t *testing.T) {
	oldAIK, newAIK := testKey(10), testKey(11)
	s := newLog(t, oldAIK)
	_ = s.Apply(signEntry(t, s, oldAIK, addDevice(1, testKey(20))))
	_ = s.Apply(signEntry(t, s, oldAIK, &vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{1}}))
	if err := s.Apply(succession(t, s, oldAIK, newAIK)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s.IdentityKey(), newAIK.Public().(ed25519.PublicKey)) {
		t.Fatal("succession did not install the new identity key")
	}
	if !bytes.Equal(s.EncryptionKey(), aekBytes(2)) {
		t.Fatal("succession did not install the new encryption key")
	}
	if s.PendingSuccession() {
		t.Fatal("succession left the revocation pending")
	}
	late := signEntry(t, s, oldAIK, addDevice(5, testKey(25)))
	if err := s.Apply(late); !errors.Is(err, vaultsync.ErrSupersededKey) {
		t.Fatalf("err = %v, want ErrSupersededKey; the revoked device still holds the old AIK", err)
	}
	if err := s.Apply(signEntry(t, s, newAIK, addDevice(5, testKey(25)))); err != nil {
		t.Fatalf("the new identity key cannot certify a device: %v", err)
	}
}

func TestDeviceLogSuccessionNeedsBothSignatures(t *testing.T) {
	oldAIK, newAIK := testKey(10), testKey(11)
	s := newLog(t, oldAIK)
	e := succession(t, s, oldAIK, newAIK)
	e.SuccessorSig[0] ^= 1
	if err := s.Apply(e); !errors.Is(err, vaultsync.ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature; the new key must prove it was present", err)
	}
}

func TestDeviceLogValidatesPayloads(t *testing.T) {
	aik := testKey(10)
	cases := map[string]*vaultsync.DeviceEntry{
		"nil cert":         {Op: vaultsync.OpAddDevice},
		"long name":        {Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{1}, SigningKey: make([]byte, 32), Name: strings.Repeat("n", 65)}},
		"long platform":    {Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{1}, SigningKey: make([]byte, 32), Platform: strings.Repeat("p", 33)}},
		"invalid utf8":     {Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{1}, SigningKey: make([]byte, 32), Name: "\xff"}},
		"short key":        {Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{Device: vaultsync.ID{1}, SigningKey: make([]byte, 31)}},
		"zero device":      {Op: vaultsync.OpAddDevice, Cert: &vaultsync.DeviceCert{SigningKey: make([]byte, 32)}},
		"revoke unknown":   {Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{9}},
		"unknown op":       {Op: 99},
		"succession size":  {Op: vaultsync.OpSuccession, NewSuite: vaultsync.Suite1, NewIdentityKey: make([]byte, 32), NewEncryptionKey: make([]byte, 10)},
		"succession suite": {Op: vaultsync.OpSuccession, NewSuite: 99, NewIdentityKey: make([]byte, 32), NewEncryptionKey: aekBytes(2)},
	}
	for name, e := range cases {
		s := newLog(t, aik)
		if err := s.Apply(signEntry(t, s, aik, e)); !errors.Is(err, vaultsync.ErrIntegrity) {
			t.Errorf("%s: err = %v, want an integrity error", name, err)
		}
	}
}

func TestDeviceLogRejectsDuplicateDevice(t *testing.T) {
	aik := testKey(10)
	s := newLog(t, aik)
	_ = s.Apply(signEntry(t, s, aik, addDevice(1, testKey(20))))
	if err := s.Apply(signEntry(t, s, aik, addDevice(1, testKey(21)))); !errors.Is(err, vaultsync.ErrDeviceLog) {
		t.Fatalf("err = %v, want ErrDeviceLog; re-certifying an id would swap a device's key", err)
	}
}

func TestDeviceLogRejectsDoubleRevocation(t *testing.T) {
	oldAIK, newAIK := testKey(10), testKey(11)
	s := newLog(t, oldAIK)
	_ = s.Apply(signEntry(t, s, oldAIK, addDevice(1, testKey(20))))
	_ = s.Apply(signEntry(t, s, oldAIK, &vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{1}}))
	_ = s.Apply(succession(t, s, oldAIK, newAIK))
	err := s.Apply(signEntry(t, s, newAIK, &vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{1}}))
	if !errors.Is(err, vaultsync.ErrDeviceLog) {
		t.Fatalf("err = %v, want ErrDeviceLog for revoking an already revoked device", err)
	}
}

func TestDeviceKeySurvivesRevocation(t *testing.T) {
	oldAIK := testKey(10)
	s := newLog(t, oldAIK)
	dsk := testKey(20)
	_ = s.Apply(signEntry(t, s, oldAIK, addDevice(1, dsk)))
	_ = s.Apply(signEntry(t, s, oldAIK, &vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: vaultsync.ID{1}}))
	key, err := s.DeviceKey(vaultsync.ID{1})
	if err != nil || !bytes.Equal(key, dsk.Public().(ed25519.PublicKey)) {
		t.Fatalf("DeviceKey after revocation = %x, %v; commits made before revocation must stay verifiable", key, err)
	}
	if !s.IsRevoked(vaultsync.ID{1}) {
		t.Fatal("IsRevoked = false after revocation")
	}
	if _, err := s.DeviceKey(vaultsync.ID{7}); !errors.Is(err, vaultsync.ErrDeviceLog) {
		t.Fatalf("DeviceKey of an uncertified device: err = %v, want ErrDeviceLog", err)
	}
}

func TestDeviceLogFailedApplyLeavesStateUnchanged(t *testing.T) {
	aik := testKey(10)
	s := newLog(t, aik)
	_ = s.Apply(signEntry(t, s, aik, addDevice(1, testKey(20))))
	seq, hash := s.Seq(), s.Hash()
	bad := signEntry(t, s, aik, addDevice(1, testKey(21)))
	_ = s.Apply(bad)
	if s.Seq() != seq || s.Hash() != hash {
		t.Fatal("a rejected entry changed seq or hash")
	}
	key, _ := s.DeviceKey(vaultsync.ID{1})
	if !bytes.Equal(key, testKey(20).Public().(ed25519.PublicKey)) {
		t.Fatal("a rejected entry replaced a device key")
	}
}

func TestNewDeviceLogStateRejectsBadKeys(t *testing.T) {
	if _, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, make([]byte, 31), aekBytes(1)); !errors.Is(err, vaultsync.ErrDeviceLog) {
		t.Errorf("short identity key: err = %v", err)
	}
	if _, err := vaultsync.NewDeviceLogState(account, vaultsync.Suite1, make([]byte, 32), make([]byte, 5)); !errors.Is(err, vaultsync.ErrDeviceLog) {
		t.Errorf("short encryption key: err = %v", err)
	}
	if _, err := vaultsync.NewDeviceLogState(account, 99, make([]byte, 32), aekBytes(1)); !errors.Is(err, vaultsync.ErrUnsupportedSuite) {
		t.Errorf("unknown suite: err = %v", err)
	}
}
