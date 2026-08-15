package domain_test

import (
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// The fingerprint is shown to a human and must be self-describing: bare base64 is
// uninterpretable a year later - nothing says what it hashes or with what.
func TestPeerFingerprintIsStableAndLabelled(t *testing.T) {
	material := []byte{1, 2, 3}
	got := domain.PeerFingerprint(material)
	if got != domain.PeerFingerprint(material) {
		t.Fatal("the fingerprint is not stable")
	}
	if !strings.HasPrefix(got, "SHA256:") {
		t.Fatalf("fingerprint %q does not name its algorithm", got)
	}
	if got == domain.PeerFingerprint([]byte{1, 2, 4}) {
		t.Fatal("different material produced the same fingerprint")
	}
}

func TestPeerFingerprintOfEmptyIsNotEmpty(t *testing.T) {
	if domain.PeerFingerprint(nil) == "" {
		t.Fatal("empty material produced an empty fingerprint; the dialog would have nothing to show")
	}
}

// The new session state must not collide with the old one: the SSH path keys off
// hostkey-required, and equal strings would merge two different dialogs into one.
func TestTrustRequiredStateIsDistinctFromHostKey(t *testing.T) {
	if domain.SessionTrustRequired == domain.SessionHostKeyRequired {
		t.Fatal("the two states are the same value")
	}
	if domain.SessionTrustRequired != "trust-required" {
		t.Fatalf("state = %q", domain.SessionTrustRequired)
	}
}

// An empty vault must yield an empty list rather than a nil a caller trips over.
func TestNewVaultDataInitialisesPeerTrust(t *testing.T) {
	data := domain.NewVaultData()
	if data.PeerTrust == nil {
		t.Fatal("PeerTrust is not initialised")
	}
	if len(data.PeerTrust) != 0 {
		t.Fatalf("PeerTrust is not empty: %v", data.PeerTrust)
	}
}

// A trust entry must know no protocol term at all: that property is the whole reason this
// mechanism is separate from known_hosts.
func TestPeerTrustEntryCarriesOpaqueMaterial(t *testing.T) {
	entry := domain.PeerTrustEntry{
		Scope:    "com.example.rdp",
		Subject:  "10.0.0.5:3389",
		Material: []byte{0x30, 0x59},
	}
	if entry.Scope == "" || entry.Subject == "" || len(entry.Material) == 0 {
		t.Fatal("the entry fields do not hold what was put in them")
	}
	if domain.MaxPeerTrustMaterial != 64*1024 {
		t.Fatalf("MaxPeerTrustMaterial = %d, want 65536", domain.MaxPeerTrustMaterial)
	}
}

// CloneVaultData lists its fields by hand. A forgotten field produces neither a build error nor
// a failing test - it simply vanishes on every vault read, and trust would stop persisting
// without a single message anywhere.
func TestCloneVaultDataCopiesPeerTrust(t *testing.T) {
	in := domain.NewVaultData()
	in.PeerTrust = []domain.PeerTrustEntry{
		{Scope: "a", Subject: "h:1", Material: []byte{1, 2, 3}},
	}
	out := domain.CloneVaultData(in)
	if len(out.PeerTrust) != 1 {
		t.Fatalf("the clone lost the trust entries: %v", out.PeerTrust)
	}
	if string(out.PeerTrust[0].Material) != string([]byte{1, 2, 3}) {
		t.Fatalf("the material was not copied: % x", out.PeerTrust[0].Material)
	}
	// The material must be a copy: a shared array would let an edit of the clone change stored
	// trust.
	out.PeerTrust[0].Material[0] = 0xFF
	if in.PeerTrust[0].Material[0] != 1 {
		t.Fatal("the clone shares its material array with the original")
	}
}
