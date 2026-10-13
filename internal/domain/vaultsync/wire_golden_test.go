package vaultsync_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

// The bytes each structure signs are a wire contract between every client version and the server:
// a change that both sides would make together - a reordered field, a flipped presence flag - still
// breaks every signature an older peer made. These digests were pinned once, when the structures
// were introduced; a failure means the layout changed and is a protocol major, never a re-pin.
var signingGolden = map[string]string{
	"envelope":     "425a942706d046d3ed0d1461585b2b82c3f31f67ee3e109d55e4d08918c801fc",
	"head":         "b15e2f67d7923b528cf2d19391d4d7ecf9b4501d2ab37f63af534cb07111f09a",
	"device-entry": "ee709a3e957a4c54bd34061add4d81dbcc8b267aac669803ce19da9e5f697b60",
	"member-op":    "b7a91013ecee078c7b683d4ef18bbb472a24bad76fee763325bd088c9418d139",
}

func signingSamples(t *testing.T) map[string][]byte {
	t.Helper()
	env := &vaultsync.Envelope{
		Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 4, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: bytes.Repeat([]byte{6}, 296), Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519,
	}
	head := &vaultsync.Head{
		Vault: vaultsync.ID{1}, Seq: 9, PrevHash: [32]byte{10}, MerkleRoot: [32]byte{11}, CurrentEpoch: 12,
		MembershipSeq: 13, MembershipHead: [32]byte{14}, Commit: vaultsync.ID{15}, Author: vaultsync.ID{16},
		ClientTimeUnixMilli: 17, SigAlg: vaultsync.SigEd25519,
	}
	entry := &vaultsync.DeviceEntry{
		Account: vaultsync.ID{18}, Seq: 19, Prev: [32]byte{20}, Op: vaultsync.OpAddDevice,
		Cert:   &vaultsync.DeviceCert{Device: vaultsync.ID{21}, SigningKey: bytes.Repeat([]byte{22}, 32), Name: "n", Platform: "p", CreatedUnixMilli: 23},
		SigAlg: vaultsync.SigEd25519,
	}
	op := &vaultsync.MemberOp{
		Vault: vaultsync.ID{24}, Seq: 25, Prev: [32]byte{26}, Kind: vaultsync.MemberRotateEpoch, Author: vaultsync.ID{27},
		AuthorDevice: vaultsync.ID{28}, Subject: vaultsync.ID{29}, Role: vaultsync.RoleEditor, Verified: true,
		Fingerprint: [32]byte{30}, Suite: vaultsync.Suite1, Epoch: 31,
		Wraps:          []vaultsync.KeyWrap{{Member: vaultsync.ID{32}, Digest: [32]byte{37}, Ciphertext: []byte{33}}},
		RevokedDevices: []vaultsync.ID{{34}}, RetentionDays: 35, RequireVerified: true, Shred: true, TimeUnixMilli: 36,
		SigAlg: vaultsync.SigEd25519,
	}
	out := map[string][]byte{}
	for name, build := range map[string]func() ([]byte, error){
		"envelope": env.SigningBytes, "head": head.SigningBytes, "device-entry": entry.SigningBytes, "member-op": op.SigningBytes,
	} {
		b, err := build()
		if err != nil {
			t.Fatal(err)
		}
		out[name] = b
	}
	return out
}

func TestSigningLayoutsAreFrozen(t *testing.T) {
	for name, msg := range signingSamples(t) {
		sum := sha256.Sum256(msg)
		if got := hex.EncodeToString(sum[:]); got != signingGolden[name] {
			t.Errorf("%s signing bytes digest = %s, want %s; the signed layout changed", name, got, signingGolden[name])
		}
	}
}
