package keys

import (
	"bytes"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

func mustGenerate(t *testing.T, algorithm string, bits int, passphrase string) *domain.KeyMaterial {
	t.Helper()
	c := NewCodec()
	km, err := c.Generate(domain.GeneratedKeySpec{Algorithm: algorithm, Bits: bits, Comment: "test"}, []byte(passphrase))
	if err != nil {
		t.Fatalf("generate %s/%d: %v", algorithm, bits, err)
	}
	return km
}

func TestGeneratedKeyIsStoredEncryptedAndNeedsItsPassphrase(t *testing.T) {
	c := NewCodec()
	km := mustGenerate(t, domain.AlgorithmEd25519, 0, "correct horse")

	if _, err := c.Unwrap(km.PEM, nil); !errors.Is(err, domain.ErrPassphraseRequired) {
		t.Errorf("unwrap without passphrase = %v, want ErrPassphraseRequired; a stored key must never be readable on its own", err)
	}
	if _, err := c.Unwrap(km.PEM, []byte("wrong")); !errors.Is(err, domain.ErrKeyPassphraseWrong) {
		t.Errorf("unwrap with wrong passphrase = %v, want ErrKeyPassphraseWrong", err)
	}
	signer, err := c.Unwrap(km.PEM, []byte("correct horse"))
	if err != nil {
		t.Fatalf("unwrap with the right passphrase: %v", err)
	}
	if got := gossh.FingerprintSHA256(signer.PublicKey()); got != km.Fingerprint {
		t.Errorf("fingerprint after round-trip = %q, want %q; the cached metadata must describe the stored key", got, km.Fingerprint)
	}
}

func TestWrapRefusesAnEmptyPassphrase(t *testing.T) {
	c := NewCodec()
	_, err := c.Generate(domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519}, nil)
	if !errors.Is(err, ErrEmptyWrapPassphrase) {
		t.Errorf("generate with no passphrase = %v, want ErrEmptyWrapPassphrase; the vault must hold no unprotected key", err)
	}
}

func TestGeneratedMetadataNamesTheAlgorithmAndSize(t *testing.T) {
	cases := []struct {
		algorithm string
		bits      int
		wantType  string
		wantBits  int
	}{
		{domain.AlgorithmEd25519, 0, domain.AlgorithmEd25519, 0},
		{domain.AlgorithmRSA, 2048, domain.AlgorithmRSA, 2048},
		{domain.AlgorithmECDSA, 256, domain.AlgorithmECDSA, 256},
		{domain.AlgorithmECDSA, 384, domain.AlgorithmECDSA, 384},
	}
	for _, tc := range cases {
		km := mustGenerate(t, tc.algorithm, tc.bits, "pw")
		if km.KeyType != tc.wantType {
			t.Errorf("%s/%d keyType = %q, want %q", tc.algorithm, tc.bits, km.KeyType, tc.wantType)
		}
		if km.Bits != tc.wantBits {
			t.Errorf("%s/%d bits = %d, want %d", tc.algorithm, tc.bits, km.Bits, tc.wantBits)
		}
		if !strings.HasPrefix(km.Fingerprint, "SHA256:") {
			t.Errorf("%s/%d fingerprint = %q, want a SHA256: prefix", tc.algorithm, tc.bits, km.Fingerprint)
		}
		if _, _, _, _, err := gossh.ParseAuthorizedKey([]byte(km.PublicKey)); err != nil {
			t.Errorf("%s/%d public key is not an authorized_keys line: %v", tc.algorithm, tc.bits, err)
		}
	}
}

func TestGenerateRejectsSizesThatWouldNotWork(t *testing.T) {
	c := NewCodec()
	cases := []struct {
		name string
		spec domain.GeneratedKeySpec
	}{
		{"rsa below the OpenSSH floor", domain.GeneratedKeySpec{Algorithm: domain.AlgorithmRSA, Bits: 1024}},
		{"rsa above the ceiling", domain.GeneratedKeySpec{Algorithm: domain.AlgorithmRSA, Bits: 8192}},
		{"ecdsa on no NIST curve", domain.GeneratedKeySpec{Algorithm: domain.AlgorithmECDSA, Bits: 512}},
		{"unknown algorithm", domain.GeneratedKeySpec{Algorithm: "dsa"}},
	}
	for _, tc := range cases {
		if _, err := c.Generate(tc.spec, []byte("pw")); !errors.Is(err, domain.ErrUnsupportedKeyAlgorithm) {
			t.Errorf("%s: err = %v, want ErrUnsupportedKeyAlgorithm", tc.name, err)
		}
	}
}

func TestNormalizeRewrapsAnUnprotectedImport(t *testing.T) {
	c := NewCodec()
	plain := mustExportUnprotected(t, mustGenerate(t, domain.AlgorithmEd25519, 0, "original"), "original")

	km, err := c.Normalize(plain, nil, []byte("vault-data-key"), "imported")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if _, err := gossh.ParsePrivateKey(km.PEM); err == nil {
		t.Error("normalized key parses with no passphrase; importing an unprotected key must not leave it unprotected")
	}
	if _, err := c.Unwrap(km.PEM, []byte("vault-data-key")); err != nil {
		t.Errorf("unwrap normalized key: %v", err)
	}
}

func TestNormalizeReportsWhatIsMissingOrWrong(t *testing.T) {
	c := NewCodec()
	protected := mustGenerate(t, domain.AlgorithmEd25519, 0, "secret")

	if _, err := c.Normalize(protected.PEM, nil, []byte("new"), ""); !errors.Is(err, domain.ErrPassphraseRequired) {
		t.Errorf("normalize without the old passphrase = %v, want ErrPassphraseRequired", err)
	}
	if _, err := c.Normalize(protected.PEM, []byte("nope"), []byte("new"), ""); !errors.Is(err, domain.ErrKeyPassphraseWrong) {
		t.Errorf("normalize with a wrong old passphrase = %v, want ErrKeyPassphraseWrong", err)
	}
}

func TestNormalizePreservesTheKeyItself(t *testing.T) {
	c := NewCodec()
	original := mustGenerate(t, domain.AlgorithmRSA, 2048, "one")

	renamed, err := c.Normalize(original.PEM, []byte("one"), []byte("two"), "new comment")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if renamed.Fingerprint != original.Fingerprint {
		t.Errorf("fingerprint changed on re-wrap: %q -> %q; re-wrapping must not produce a different key", original.Fingerprint, renamed.Fingerprint)
	}
	if bytes.Equal(renamed.PEM, original.PEM) {
		t.Error("re-wrapped bytes are identical; the new passphrase was not applied")
	}
}

func TestExportProducesAKeyOpenSSHCanRead(t *testing.T) {
	c := NewCodec()
	km := mustGenerate(t, domain.AlgorithmEd25519, 0, "stored")

	protected, err := c.Export(km.PEM, []byte("stored"), []byte("chosen"), "exported")
	if err != nil {
		t.Fatalf("export protected: %v", err)
	}
	if _, err := gossh.ParsePrivateKeyWithPassphrase(protected, []byte("chosen")); err != nil {
		t.Errorf("exported key does not open with the chosen passphrase: %v", err)
	}
	if _, err := gossh.ParsePrivateKey(protected); err == nil {
		t.Error("exported key opens with no passphrase despite one being chosen")
	}

	bare, err := c.Export(km.PEM, []byte("stored"), nil, "exported")
	if err != nil {
		t.Fatalf("export unprotected: %v", err)
	}
	signer, err := gossh.ParsePrivateKey(bare)
	if err != nil {
		t.Fatalf("unprotected export does not parse: %v", err)
	}
	if got := gossh.FingerprintSHA256(signer.PublicKey()); got != km.Fingerprint {
		t.Errorf("exported key fingerprint = %q, want %q", got, km.Fingerprint)
	}
}

func TestExportRefusesTheWrongStoredPassphrase(t *testing.T) {
	c := NewCodec()
	km := mustGenerate(t, domain.AlgorithmEd25519, 0, "stored")
	if _, err := c.Export(km.PEM, []byte("guess"), nil, ""); !errors.Is(err, domain.ErrKeyPassphraseWrong) {
		t.Errorf("export with a wrong stored passphrase = %v, want ErrKeyPassphraseWrong", err)
	}
}

func TestDescribeAnswersWithoutThePassphrase(t *testing.T) {
	c := NewCodec()
	cases := []struct {
		name          string
		pem           []byte
		wantType      string
		wantEncrypted bool
	}{
		{
			name:          "encrypted openssh key",
			pem:           mustGenerate(t, domain.AlgorithmEd25519, 0, "pw").PEM,
			wantType:      "openssh",
			wantEncrypted: true,
		},
		{
			name:          "unprotected openssh key",
			pem:           mustExportUnprotected(t, mustGenerate(t, domain.AlgorithmRSA, 2048, "pw"), "pw"),
			wantType:      domain.AlgorithmRSA,
			wantEncrypted: false,
		},
		{
			name:          "not a key at all",
			pem:           []byte("hello"),
			wantType:      "unknown",
			wantEncrypted: false,
		},
	}
	for _, tc := range cases {
		gotType, gotEncrypted := c.Describe(tc.pem)
		if gotType != tc.wantType || gotEncrypted != tc.wantEncrypted {
			t.Errorf("%s: describe = (%q, %v), want (%q, %v)", tc.name, gotType, gotEncrypted, tc.wantType, tc.wantEncrypted)
		}
	}
}

func TestDescribeRecognisesLegacyPEMHeaders(t *testing.T) {
	// A v3 vault can hold a key in the shape ssh-keygen wrote before OpenSSH became the default
	// container. Describe has to answer for those too, since migration reads every stored key
	// before anything re-wraps it.
	legacy := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-128-CBC,0011"},
		Bytes:   []byte("not-real-key-bytes"),
	})
	gotType, gotEncrypted := c.Describe(legacy)
	if gotType != domain.AlgorithmRSA || !gotEncrypted {
		t.Errorf("describe legacy encrypted RSA = (%q, %v), want (%q, true)", gotType, gotEncrypted, domain.AlgorithmRSA)
	}
}

var c = NewCodec()

func TestNewDataKeyIsRandomAndFullLength(t *testing.T) {
	first, err := NewDataKey()
	if err != nil {
		t.Fatalf("new data key: %v", err)
	}
	second, err := NewDataKey()
	if err != nil {
		t.Fatalf("new data key: %v", err)
	}
	if len(first) != DefaultDataKeyBytes {
		t.Errorf("data key length = %d, want %d", len(first), DefaultDataKeyBytes)
	}
	if bytes.Equal(first, second) {
		t.Error("two data keys are identical; every vault-policy key must get its own")
	}
}

// mustExportUnprotected produces the shape a user's ~/.ssh/id_* file has before import.
func mustExportUnprotected(t *testing.T, km *domain.KeyMaterial, passphrase string) []byte {
	t.Helper()
	out, err := NewCodec().Export(km.PEM, []byte(passphrase), nil, "")
	if err != nil {
		t.Fatalf("export unprotected: %v", err)
	}
	return out
}
