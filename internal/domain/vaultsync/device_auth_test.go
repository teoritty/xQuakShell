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
