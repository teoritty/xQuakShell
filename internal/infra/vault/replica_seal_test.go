package vault

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"filippo.io/age"

	"xquakshell/internal/domain"
)

func sampleDoc() domain.ReplicaDocument {
	return domain.ReplicaDocument{
		Scope:   "com.example.sync",
		Version: domain.VersionVector{"device-a": 2},
		Folders: []domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
		Connections: []domain.Connection{
			{ID: "c1", FolderID: "sync", Host: "prod.example.com", Port: 22},
		},
		Passwords: map[string]domain.PasswordBlob{"p1": {Value: []byte("hunter2")}},
	}
}

// The round trip is the whole contract: what one device seals, another opens with the same key and
// gets back unchanged.
func TestASealedReplicaOpensBackToWhatWentIn(t *testing.T) {
	sealer := NewReplicaSealer()
	doc := sampleDoc()

	sealed, err := sealer.Seal(doc, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := sealer.Open(sealed, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got.Scope != doc.Scope || got.Version["device-a"] != 2 {
		t.Fatalf("scope or version changed: %+v", got)
	}
	if len(got.Connections) != 1 || got.Connections[0].Host != "prod.example.com" {
		t.Fatalf("connections changed: %+v", got.Connections)
	}
	if !bytes.Equal(got.Passwords["p1"].Value, []byte("hunter2")) {
		t.Fatalf("the password did not survive: %q", got.Passwords["p1"].Value)
	}
}

// T1, checked rather than asserted. The transport is assumed hostile, so nothing it carries may be
// readable - and hostnames and usernames are intelligence, not "just config".
func TestNothingReadableSurvivesInTheSealedBytes(t *testing.T) {
	sealer := NewReplicaSealer()

	sealed, err := sealer.Seal(sampleDoc(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	for _, secret := range []string{"prod.example.com", "hunter2", "com.example.sync", "device-a", "Sync"} {
		if bytes.Contains(sealed, []byte(secret)) {
			t.Errorf("%q appears in the sealed bytes", secret)
		}
	}
}

// The wrong key does not open it, and says so with the same error a corrupted payload gets. Telling
// the two apart would hand a guesser the one bit that says whether the key is close.
func TestTheWrongKeyAndACorruptPayloadFailAlike(t *testing.T) {
	sealer := NewReplicaSealer()
	sealed, err := sealer.Seal(sampleDoc(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	_, wrongKey := sealer.Open(sealed, "wrong key entirely")
	_, corrupt := sealer.Open([]byte("not a sealed replica at all"), "correct horse battery staple")

	for name, err := range map[string]error{"wrong key": wrongKey, "corrupt payload": corrupt} {
		if !errors.Is(err, domain.ErrReplicaOpenFailed) {
			t.Errorf("%s: err = %v, want ErrReplicaOpenFailed", name, err)
		}
	}
}

// Sealing to an empty key would produce something anyone can open, which is worse than refusing:
// the caller would believe the payload was protected.
func TestSealingWithoutAKeyIsRefused(t *testing.T) {
	sealer := NewReplicaSealer()

	if _, err := sealer.Seal(sampleDoc(), ""); !errors.Is(err, domain.ErrReplicaKeyRequired) {
		t.Fatalf("Seal err = %v, want ErrReplicaKeyRequired", err)
	}
	if _, err := sealer.Open([]byte("anything"), ""); !errors.Is(err, domain.ErrReplicaKeyRequired) {
		t.Fatalf("Open err = %v, want ErrReplicaKeyRequired", err)
	}
}

// Two seals of the same document must differ. Identical bytes would tell a server watching the
// endpoint that nothing changed between two pushes, which is a fact about the user's work it has no
// business learning.
func TestSealingTwiceProducesDifferentBytes(t *testing.T) {
	sealer := NewReplicaSealer()

	first, err := sealer.Seal(sampleDoc(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	second, err := sealer.Seal(sampleDoc(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same document produced identical bytes")
	}
}

// An empty document is a legitimate thing to send - a scope the user has not put anything in yet -
// and it must survive the trip as empty rather than as an error.
func TestAnEmptyReplicaSealsAndOpens(t *testing.T) {
	sealer := NewReplicaSealer()
	empty := domain.ReplicaDocument{Scope: "com.example.sync"}

	sealed, err := sealer.Seal(empty, "key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := sealer.Open(sealed, "key")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got.Scope != "com.example.sync" || len(got.Connections) != 0 {
		t.Fatalf("an empty document came back as %+v", got)
	}
}

// The sealer is what the usecase layer holds through the domain port, so it has to satisfy it.
func TestTheSealerSatisfiesTheDomainPort(t *testing.T) {
	var _ domain.ReplicaSealer = NewReplicaSealer()
}

// Sealed bytes carry no plaintext marker a server could sort or filter on. The age header is
// unavoidable and says only that this is an encrypted file, which every payload here is.
func TestTheSealedFormNamesNoScope(t *testing.T) {
	sealer := NewReplicaSealer()

	sealed, err := sealer.Seal(sampleDoc(), "key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if strings.Contains(string(sealed), "scope") {
		t.Fatal("the sealed form names the scope, so a server can tell payloads apart by plugin")
	}
}

// Bytes that decrypt but are not a replica must fail cleanly rather than produce a zero document.
// A zero document names no scope and holds nothing, which the merge would read as "the other device
// deleted everything" - the one message a malformed payload must not be able to send.
func TestAPayloadThatDecryptsButIsNotAReplicaIsRefused(t *testing.T) {
	const key = "correct horse battery staple"
	recipient, err := age.NewScryptRecipient(key)
	if err != nil {
		t.Fatalf("recipient: %v", err)
	}
	recipient.SetWorkFactor(scryptWorkFactor)
	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := writer.Write([]byte("this is not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := NewReplicaSealer().Open(buf.Bytes(), key); !errors.Is(err, domain.ErrReplicaOpenFailed) {
		t.Fatalf("err = %v, want ErrReplicaOpenFailed", err)
	}
}

// The work factor is a security constant and nothing else in the suite would notice it being
// lowered: a weaker seal produces bytes that open exactly the same way. age writes it into the
// scrypt stanza in the clear, so it can be read back and pinned.
func TestTheSealDeclaresTheWorkFactorTheVaultUses(t *testing.T) {
	sealed, err := NewReplicaSealer().Seal(sampleDoc(), "key")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	header, _, found := strings.Cut(string(sealed), "\n--- ")
	if !found {
		t.Fatalf("no age header in the sealed bytes: %q", sealed[:min(120, len(sealed))])
	}
	want := fmt.Sprintf(" %d\n", scryptWorkFactor)
	if !strings.Contains(header, want) {
		t.Fatalf("the header does not declare work factor %d:\n%s", scryptWorkFactor, header)
	}
}
