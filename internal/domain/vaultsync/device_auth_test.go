package vaultsync_test

import (
	"bytes"
	"regexp"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

func TestDeviceAuthMessageBindsEverything(t *testing.T) {
	base, _ := vaultsync.DeviceAuthMessage(vaultsync.ID{1}, vaultsync.ID{2}, []byte("nonce"))
	for name, msg := range map[string][]byte{
		"account": must(vaultsync.DeviceAuthMessage(vaultsync.ID{9}, vaultsync.ID{2}, []byte("nonce"))),
		"device":  must(vaultsync.DeviceAuthMessage(vaultsync.ID{1}, vaultsync.ID{9}, []byte("nonce"))),
		"nonce":   must(vaultsync.DeviceAuthMessage(vaultsync.ID{1}, vaultsync.ID{2}, []byte("other"))),
	} {
		if bytes.Equal(msg, base) {
			t.Errorf("changing the %s left the device proof unchanged; a proof could be replayed", name)
		}
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestAccountFingerprintFormat(t *testing.T) {
	fp := vaultsync.AccountFingerprint(bytes.Repeat([]byte{7}, 32))
	if !regexp.MustCompile(`^(\d{5} ){11}\d{5}$`).MatchString(fp) {
		t.Fatalf("fingerprint %q is not twelve groups of five digits", fp)
	}
	if fp == vaultsync.AccountFingerprint(bytes.Repeat([]byte{8}, 32)) {
		t.Fatal("two keys share a fingerprint")
	}
	if fp != vaultsync.AccountFingerprint(bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("a fingerprint is not stable for one key")
	}
}

func TestServerTLSMessageIsLabelled(t *testing.T) {
	a, _ := vaultsync.ServerTLSMessage([]byte("spki"))
	b, _ := vaultsync.DeviceAuthMessage(vaultsync.ID{}, vaultsync.ID{}, []byte("spki"))
	if bytes.Equal(a, b) {
		t.Fatal("the TLS key signature shares bytes with a device proof")
	}
	c, _ := vaultsync.ServerTLSMessage([]byte("other"))
	if bytes.Equal(a, c) {
		t.Fatal("the TLS key signature ignores the key")
	}
}

func TestRekeyMessageBindsAccountRecordAndBundle(t *testing.T) {
	base, _ := vaultsync.RekeyMessage(vaultsync.ID{1}, []byte("rec"), []byte("bundle"))
	for name, other := range map[string]func() ([]byte, error){
		"account": func() ([]byte, error) {
			return vaultsync.RekeyMessage(vaultsync.ID{2}, []byte("rec"), []byte("bundle"))
		},
		"record": func() ([]byte, error) {
			return vaultsync.RekeyMessage(vaultsync.ID{1}, []byte("rek"), []byte("bundle"))
		},
		"bundle": func() ([]byte, error) {
			return vaultsync.RekeyMessage(vaultsync.ID{1}, []byte("rec"), []byte("bundlf"))
		},
		"shift": func() ([]byte, error) {
			return vaultsync.RekeyMessage(vaultsync.ID{1}, []byte("recb"), []byte("undle"))
		},
	} {
		got, _ := other()
		if bytes.Equal(base, got) {
			t.Errorf("a different %s signs the same bytes; one consent would replace another bundle", name)
		}
	}
	tls, _ := vaultsync.ServerTLSMessage([]byte("rec"))
	if bytes.HasPrefix(base, tls[:8]) {
		t.Error("the rekey message shares its label with another signature")
	}
}
