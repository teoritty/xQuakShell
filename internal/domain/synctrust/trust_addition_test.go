package synctrust_test

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- builds OpenSSH's hashed known_hosts form, which is HMAC-SHA1
	"encoding/base64"
	"encoding/json"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// hashedHost writes host the way OpenSSH does with HashKnownHosts on.
func hashedHost(host string, salt byte) string {
	s := []byte{salt, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	mac := hmac.New(sha1.New, s)
	mac.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString(s) + "|" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func heldHosts(t *testing.T, lines ...string) map[vaultsync.ID]synctrust.Entry {
	t.Helper()
	out := map[vaultsync.ID]synctrust.Entry{}
	for i, l := range lines {
		out[vaultsync.ID{byte(i + 1)}] = synctrust.Entry{Type: synctrust.TypeKnownHost, Key: l, Data: raw(t, map[string]string{"line": l})}
	}
	return out
}

func TestClassifyAdditionHoldsASecondLineForAKnownHost(t *testing.T) {
	const stored = "web.example ssh-ed25519 AAAAstored"
	cases := []struct {
		name     string
		held     []string
		incoming string
		want     synctrust.Action
	}{
		{"a host this device has never seen", []string{stored}, "db.example ssh-ed25519 AAAAnew", synctrust.ActionApply},
		{"nothing held", nil, "web.example ssh-ed25519 AAAAnew", synctrust.ActionApply},
		{"another key type for the host", []string{stored}, "web.example ssh-rsa AAAAnew", synctrust.ActionPending},
		{"the host in hashed form", []string{stored}, hashedHost("web.example", 1) + " ssh-ed25519 AAAAnew", synctrust.ActionPending},
		{"a hashed stored host, the new line plain", []string{hashedHost("web.example", 1) + " ssh-ed25519 AAAAstored"}, "web.example ssh-rsa AAAAnew", synctrust.ActionPending},
		{"a hashed stored host, another host plain", []string{hashedHost("web.example", 1) + " ssh-ed25519 AAAAstored"}, "db.example ssh-rsa AAAAnew", synctrust.ActionApply},
		{"a hashed host beside a hashed host", []string{hashedHost("db.example", 1) + " ssh-ed25519 AAAAstored"}, hashedHost("web.example", 2) + " ssh-ed25519 AAAAnew", synctrust.ActionPending},
		{"a marker in front of the host", []string{stored}, "@revoked web.example ssh-ed25519 AAAAnew", synctrust.ActionPending},
		{"the host first in a host list", []string{stored}, "web.example,10.0.0.1 ssh-ed25519 AAAAnew", synctrust.ActionPending},
		{"a stored host list", []string{"web.example,10.0.0.1 ssh-ed25519 AAAAstored"}, "web.example ssh-rsa AAAAnew", synctrust.ActionPending},
	}
	for _, c := range cases {
		v, err := synctrust.ClassifyAddition(synctrust.TypeKnownHost, heldHosts(t, c.held...), raw(t, map[string]string{"line": c.incoming}))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if v.Action != c.want {
			t.Errorf("%s: action %v, want %v; a line the known_hosts lookup would read for a stored host must wait like a replaced key", c.name, v.Action, c.want)
		}
		if v.Action != synctrust.ActionApply && v.Reason == "" {
			t.Errorf("%s: a held change carries no reason to show the user", c.name)
		}
	}
}

func TestClassifyAdditionLeavesOtherTypesAlone(t *testing.T) {
	held := heldHosts(t, "web.example ssh-ed25519 AAAAstored")
	v, err := synctrust.ClassifyAddition(synctrust.TypeFolder, held, raw(t, map[string]string{"id": "f"}))
	if err != nil || v.Action != synctrust.ActionApply {
		t.Fatalf("folder: %v, %v; only known hosts are compared with other records", v, err)
	}
}

func TestClassifyAdditionRefusesUndecodableData(t *testing.T) {
	if _, err := synctrust.ClassifyAddition(synctrust.TypeKnownHost, nil, json.RawMessage(`{"line":1}`)); err == nil {
		t.Fatal("an undecodable known host was classified; it must be quarantined instead")
	}
}
