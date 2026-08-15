package wails

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// The DTO is the boundary the key material must not cross. It is checked on the serialized form
// rather than on the struct, because that is what actually reaches a renderer: a field added later
// without a json tag would still ship.
func TestPeerTrustDTOCarriesNoMaterial(t *testing.T) {
	added := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	material := []byte("secret-peer-material")
	dtos := PeerTrustToDTO([]domain.PeerTrustEntry{
		{Scope: "com.example.plugin", Subject: "10.0.0.5:3389", Material: material, AddedAt: added},
	})
	if len(dtos) != 1 {
		t.Fatalf("dtos = %d, want 1", len(dtos))
	}

	encoded, err := json.Marshal(dtos[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "secret-peer-material") {
		t.Fatalf("the material reached the UI payload: %s", encoded)
	}
	if dtos[0].Fingerprint != domain.PeerFingerprint(material) {
		t.Fatalf("fingerprint = %q, want the one the trust dialog renders", dtos[0].Fingerprint)
	}
	if dtos[0].AddedAt != "2026-08-01T12:00:00Z" {
		t.Fatalf("addedAt = %q, want an RFC3339 instant", dtos[0].AddedAt)
	}
}

// An entry written by an older build carries no timestamp. Rendering the zero time would tell the
// user this identity was trusted in year 1.
func TestPeerTrustDTOLeavesAnUnknownDateEmpty(t *testing.T) {
	dtos := PeerTrustToDTO([]domain.PeerTrustEntry{{Scope: "p", Subject: "h:1", Material: []byte{1}}})
	if dtos[0].AddedAt != "" {
		t.Fatalf("addedAt = %q, want empty for an entry with no recorded date", dtos[0].AddedAt)
	}
}

// An empty list must serialize as [] rather than null: the UI iterates it directly.
func TestPeerTrustToDTOIsNeverNil(t *testing.T) {
	encoded, err := json.Marshal(PeerTrustToDTO(nil))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("encoded = %s, want []", encoded)
	}
}

// Every peer trust handler refuses before it reaches a nil service. Without this the window would
// panic on a build where the plugin runtime never came up.
func TestPeerTrustHandlersRefuseWithoutTheService(t *testing.T) {
	api := &AppAPI{}
	if err := api.ResolvePeerTrust("s1", "trust", "SHA256:x"); err == nil {
		t.Fatal("ResolvePeerTrust accepted a decision with no service behind it")
	}
	if _, err := api.GetPeerTrust(); err == nil {
		t.Fatal("GetPeerTrust answered with no service behind it")
	}
	if err := api.RemovePeerTrust("p", "h:1"); err == nil {
		t.Fatal("RemovePeerTrust answered with no service behind it")
	}
}
