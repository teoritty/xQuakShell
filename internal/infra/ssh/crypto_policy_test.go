package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"slices"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// The point of pinning the policy is that a dependency bump cannot quietly put a broken algorithm
// back on the wire. These names are therefore asserted as absent, one by one: a list that merely
// "looks modern" is not a check, and the failure has to say which algorithm came back.
func TestCryptoPolicyExcludesBrokenAlgorithms(t *testing.T) {
	tests := []struct {
		name    string
		list    []string
		banned  []string
		because string
	}{
		{
			name:   "host key algorithms",
			list:   hostKeyAlgorithms,
			// These identifiers are deprecated upstream, which is exactly why they are named here:
			// the assertion is that they never reappear in the policy.
			//nolint:staticcheck // SA1019: naming the withdrawn algorithms is the assertion
			banned: []string{gossh.KeyAlgoRSA, gossh.InsecureKeyAlgoDSA, gossh.InsecureCertAlgoDSAv01, gossh.CertAlgoRSAv01},
			// ssh-rsa is RSA-with-SHA-1; OpenSSH disabled it by default in 8.8.
			because: "it signs with SHA-1",
		},
		{
			name:    "key exchanges",
			list:    keyExchanges,
			banned:  []string{gossh.InsecureKeyExchangeDH1SHA1, gossh.InsecureKeyExchangeDH14SHA1, gossh.InsecureKeyExchangeDHGEXSHA1},
			because: "it is a SHA-1 exchange",
		},
		{
			name:    "ciphers",
			list:    ciphers,
			banned:  []string{gossh.InsecureCipherAES128CBC, gossh.InsecureCipherTripleDESCBC, gossh.InsecureCipherRC4, gossh.InsecureCipherRC4128, gossh.InsecureCipherRC4256},
			because: "CBC and RC4 have no place in a new connection",
		},
		{
			name:    "MACs",
			list:    macs,
			banned:  []string{gossh.HMACSHA1, gossh.InsecureHMACSHA196},
			because: "it is SHA-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, banned := range tt.banned {
				if slices.Contains(tt.list, banned) {
					t.Errorf("%s contains %q; %s", tt.name, banned, tt.because)
				}
			}
		})
	}
}

// A policy that excluded everything would also pass the test above while making the client unable
// to connect to anything. These are the algorithms a current OpenSSH server offers.
func TestCryptoPolicyKeepsWhatCurrentServersOffer(t *testing.T) {
	required := map[string][]string{
		"host key algorithms": {gossh.KeyAlgoED25519, gossh.KeyAlgoRSASHA512, gossh.KeyAlgoECDSA256},
		"key exchanges":       {gossh.KeyExchangeCurve25519, gossh.KeyExchangeECDHP256},
		"ciphers":             {gossh.CipherChaCha20Poly1305, gossh.CipherAES256GCM, gossh.CipherAES256CTR},
		"MACs":                {gossh.HMACSHA256ETM, gossh.HMACSHA256},
	}
	lists := map[string][]string{
		"host key algorithms": hostKeyAlgorithms,
		"key exchanges":       keyExchanges,
		"ciphers":             ciphers,
		"MACs":                macs,
	}
	for name, want := range required {
		for _, algo := range want {
			if !slices.Contains(lists[name], algo) {
				t.Errorf("%s is missing %q; a current OpenSSH server would fail to negotiate", name, algo)
			}
		}
	}
}

// applyCryptoPolicy has to actually reach the config. Leaving any of the four nil hands that axis
// back to the library default, which is the situation this replaced.
func TestApplyCryptoPolicySetsEveryAxis(t *testing.T) {
	cfg := &gossh.ClientConfig{}
	applyCryptoPolicy(cfg)

	if len(cfg.HostKeyAlgorithms) == 0 {
		t.Error("HostKeyAlgorithms left unset; host key negotiation falls back to the library default")
	}
	if len(cfg.KeyExchanges) == 0 {
		t.Error("KeyExchanges left unset")
	}
	if len(cfg.Ciphers) == 0 {
		t.Error("Ciphers left unset")
	}
	if len(cfg.MACs) == 0 {
		t.Error("MACs left unset")
	}
}

func rsaHostKey(t *testing.T, bits int) gossh.PublicKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generate %d-bit RSA key: %v", bits, err)
	}
	pub, err := gossh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("wrap RSA public key: %v", err)
	}
	return pub
}

// Algorithm negotiation cannot catch this: rsa-sha2-256 is a sound algorithm and says nothing
// about the modulus signing with it, so a 1024-bit host key arrives over a policy that correctly
// refused ssh-rsa. Trusting one on first contact records a key an attacker can factor as the thing
// every later connection is checked against.
func TestCheckHostKeyStrengthRejectsUndersizedRSA(t *testing.T) {
	if err := checkHostKeyStrength(rsaHostKey(t, 1024)); !errors.Is(err, ErrWeakHostKey) {
		t.Fatalf("checkHostKeyStrength(1024-bit RSA) = %v, want ErrWeakHostKey", err)
	}
}

func TestCheckHostKeyStrengthAcceptsRSAAtTheFloor(t *testing.T) {
	if err := checkHostKeyStrength(rsaHostKey(t, MinRSAHostKeyBits)); err != nil {
		t.Fatalf("checkHostKeyStrength(%d-bit RSA) = %v, want nil", MinRSAHostKeyBits, err)
	}
}

// Ed25519 has one size, fixed by its name. Measuring it would be meaningless and refusing it would
// break the algorithm this policy prefers above all others.
func TestCheckHostKeyStrengthAcceptsEd25519(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkHostKeyStrength(sshPub); err != nil {
		t.Fatalf("checkHostKeyStrength(ed25519) = %v, want nil", err)
	}
}

// The weak-key refusal must reach the caller as a refusal, not as an unknown host: answering the
// trust prompt would otherwise record the weak key.
func TestHostKeyCallbackRejectsAWeakKeyBeforeConsultingKnownHosts(t *testing.T) {
	repo := &countingKnownHosts{}
	callback := NewHostKeyChecker(repo).HostKeyCallback()

	err := callback("example.com:22", nil, rsaHostKey(t, 1024))

	if !errors.Is(err, ErrWeakHostKey) {
		t.Fatalf("HostKeyCallback err = %v, want ErrWeakHostKey", err)
	}
	if repo.checks != 0 {
		t.Errorf("known_hosts was consulted %d times for a key that is refused outright", repo.checks)
	}
	if !strings.Contains(err.Error(), "example.com") {
		t.Errorf("error %q does not name the host it refers to", err.Error())
	}
}

// countingKnownHosts records whether the trust lookup was reached at all, so a test can show the
// weak-key refusal happens before it rather than instead of a match.
type countingKnownHosts struct {
	checks int
	err    error
}

func (c *countingKnownHosts) Check(string, gossh.PublicKey) error {
	c.checks++
	return c.err
}
func (c *countingKnownHosts) Add(context.Context, string, gossh.PublicKey) error     { return nil }
func (c *countingKnownHosts) List() ([]domain.KnownHostEntry, error)                 { return nil, nil }
func (c *countingKnownHosts) Remove(context.Context, string) error                   { return nil }
func (c *countingKnownHosts) Replace(context.Context, string, gossh.PublicKey) error { return nil }
