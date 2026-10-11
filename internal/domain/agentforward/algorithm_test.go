package agentforward

import (
	"errors"
	"testing"
)

func TestSigningAlgorithmAcceptsMatchingCombinations(t *testing.T) {
	cases := []struct {
		key, algorithm string
		flags          Flags
	}{
		{"ssh-ed25519", "ssh-ed25519", 0},
		{"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp256", 0},
		{"ecdsa-sha2-nistp384", "ecdsa-sha2-nistp384", 0},
		{"ecdsa-sha2-nistp521", "ecdsa-sha2-nistp521", 0},
		{"ssh-rsa", "rsa-sha2-256", FlagRSASHA256},
		{"ssh-rsa", "rsa-sha2-512", FlagRSASHA512},
	}
	for _, c := range cases {
		got, err := SigningAlgorithm(c.key, c.algorithm, c.flags)
		if err != nil || got != c.algorithm {
			t.Errorf("SigningAlgorithm(%s, %s, %d) = %q, %v; want %q", c.key, c.algorithm, c.flags, got, err, c.algorithm)
		}
	}
}

// Every refusal here is a request the agent must not sign: either it asks for SHA-1, or the
// flags and the login data disagree about the algorithm, which means the signature would not be
// the one the login data claims.
func TestSigningAlgorithmRefusesMismatchesAndSHA1(t *testing.T) {
	cases := map[string]struct {
		key, algorithm string
		flags          Flags
	}{
		"rsa sha1":                    {"ssh-rsa", "ssh-rsa", 0},
		"rsa sha1 with sha256 flag":   {"ssh-rsa", "ssh-rsa", FlagRSASHA256},
		"rsa sha256 without flag":     {"ssh-rsa", "rsa-sha2-256", 0},
		"rsa sha512 without flag":     {"ssh-rsa", "rsa-sha2-512", 0},
		"rsa sha256 with sha512 flag": {"ssh-rsa", "rsa-sha2-256", FlagRSASHA512},
		"rsa sha512 with sha256 flag": {"ssh-rsa", "rsa-sha2-512", FlagRSASHA256},
		"rsa both flags":              {"ssh-rsa", "rsa-sha2-512", FlagRSASHA256 | FlagRSASHA512},
		"ed25519 with rsa flag":       {"ssh-ed25519", "ssh-ed25519", FlagRSASHA256},
		"ed25519 named as rsa":        {"ssh-ed25519", "rsa-sha2-256", FlagRSASHA256},
		"ecdsa curve mismatch":        {"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", 0},
		"reserved flag":               {"ssh-ed25519", "ssh-ed25519", flagReserved},
		"unknown flag bit":            {"ssh-ed25519", "ssh-ed25519", 1 << 7},
		"dsa":                         {"ssh-dss", "ssh-dss", 0},
		"security key":                {"sk-ssh-ed25519@openssh.com", "sk-ssh-ed25519@openssh.com", 0},
		"certificate":                 {"ssh-ed25519-cert-v01@openssh.com", "ssh-ed25519-cert-v01@openssh.com", 0},
		"empty":                       {"", "", 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SigningAlgorithm(c.key, c.algorithm, c.flags); !errors.Is(err, ErrAlgorithmRefused) {
				t.Fatalf("err = %v, want ErrAlgorithmRefused", err)
			}
		})
	}
}
