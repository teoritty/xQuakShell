package agentforward

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestAuthorizedKeyBlobDecodesTheBlob(t *testing.T) {
	blob := keyBlob("ssh-ed25519")
	line := "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " deploy@laptop"
	got, err := AuthorizedKeyBlob(line)
	if err != nil || !bytes.Equal(got, blob) {
		t.Fatalf("AuthorizedKeyBlob = %x, %v; want the encoded blob", got, err)
	}
	if got, err := AuthorizedKeyBlob("ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob)); err != nil || !bytes.Equal(got, blob) {
		t.Errorf("a line without a comment must decode too: %v", err)
	}
}

func TestAuthorizedKeyBlobRefusesMalformedLines(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString(keyBlob("ssh-ed25519"))
	for name, line := range map[string]string{
		"empty":          "",
		"type only":      "ssh-ed25519",
		"bad base64":     "ssh-ed25519 !!!",
		"type mismatch":  "ssh-rsa " + blob,
		"truncated blob": "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 40, 's'}),
	} {
		if _, err := AuthorizedKeyBlob(line); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err = %v, want ErrInvalidConfig", name, err)
		}
	}
}

func TestConfigCloneSharesNoMemory(t *testing.T) {
	c := &Config{Enabled: true, IdentityIDs: []string{"a"}, Confirm: ConfirmSessionWindow, WindowMinutes: 5}
	clone := c.Clone()
	if clone == c || clone.Confirm != c.Confirm || clone.WindowMinutes != 5 || !clone.Enabled {
		t.Fatalf("Clone must be a distinct copy with the same values: %+v", clone)
	}
	clone.IdentityIDs[0] = "b"
	if c.IdentityIDs[0] != "a" {
		t.Error("the clone's key list shares the original's array")
	}
	var nilCfg *Config
	if nilCfg.Clone() != nil {
		t.Error("cloning no configuration must give no configuration")
	}
}
